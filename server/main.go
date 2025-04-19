package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	"paste.org.cn/paste/server/db"
	"paste.org.cn/paste/server/middleware"
	"paste.org.cn/paste/server/router"
	"paste.org.cn/paste/server/util"
)

func main() {
	// 设置go运行时使用所有的CPU核心，以提高并发能力
	runtime.GOMAXPROCS(runtime.NumCPU())

	// 创建一个带cancel方法的上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 加载配置文件
	util.LoadConfig("config")

	// 初始化 Limits 配置
	util.InitializeLimits()

	// 初始化 图片存储 配置
	util.InitializeStorage()

	// 检查 paste.mgo 是否存在
	if !viper.IsSet("paste.mgo") {
		log.Fatalf("paste.mgo is not set in the configuration")
	}

	fmt.Println("test2")

	// 创建上传目录
	if err := os.MkdirAll("uploads", 0755); err != nil {
		log.Fatalf("Failed to create upload directory: %v", err)
	}

	// 注入中间件
	paste := gin.New()
	paste.Use(gin.Recovery()) // gin.Recovery 是gin自带中间件，用于捕获panic并返回500错误
	paste.Use(middleware.LogInfo)
	paste.Use(middleware.ReqID)

	// 添加静态文件服务
	paste.Static("/uploads", "./uploads")

	// 初始化数据库
	pasteDB, err := db.NewPaste(ctx, viper.Sub("paste.mgo")) //viper.Sub从全局配置中提取键为“paste.mgo"的部分，并返回一个新的viper实例
	if err != nil {
		log.Errorf("init paste db failed: %+v", err)
		return
	}
	// 初始化路由
	router.Init(paste, pasteDB)

	// 初始化图片清理器
	imageCleaner := util.NewImageCleaner(pasteDB.GetCollection(), 1*time.Hour)
	imageCleaner.Start()
	defer imageCleaner.Stop()

	// 创建服务器
	srv := &http.Server{
		Addr:    util.GetServerHost(viper.GetString("server.host")),
		Handler: paste,
	}
	// 启动服务器
	go util.RunServer(srv)

	// 创建通道，用于接收操作系统信号
	quit := make(chan os.Signal, 5)
	// 将指定的信号（SIGINT 和 SIGTERM）转发到通道 quit
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	// 阻塞等待接收信号
	<-quit
	log.Println("Shutdown Server ...")

	// 在接收到关闭信号后取消上下文，让所有与 ctx 相关的 goroutine 停止工作
	cancel()

	// 优雅地关闭服务器
	util.ShutdownServer(srv)
}
