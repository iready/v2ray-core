package main

import (
	"context"
	core "github.com/v2fly/v2ray-core/v5"
	pb "github.com/v2fly/v2ray-core/v5/main/z/proto"
	"github.com/v2fly/v2ray-core/v5/main/z/rocket"
	"log"
	"time"
)

var (
	nowVersion  int32  = 0
	waitSeconds int32  = 15
	Address     string = "127.0.0.1:49901"
	Key         string = "liangxian"
)

// 启动
func launch() {
	rs := rocket.RS{
		Address:      Address,
		Key:          Key,
		ServerStatus: pb.ServerStatus_READY,
	}
	ctx := context.Background()
	config := rs.LoopFetch(ctx, waitSeconds)
	nowVersion = config.GetVersion()
	log.Printf("获取配置成功当前版本为%v", nowVersion)
	go start(ctx, &rs, config)
	watchConfig(ctx, &rs)
}

func start(ctx context.Context, rs *rocket.RS, conf *pb.GetConfigRes) {
	rs.ServerStatus = pb.ServerStatus_STARTING
	if rs.ServerCancel != nil {
		rs.ServerCancel()
	}
	if rs.V2flyServer != nil {
		err := rs.V2flyServer.Close()
		if err != nil {
			log.Printf("关闭失败：%v", err)
		}
	}
	_, cancelFunc := context.WithCancel(ctx)
	rs.ServerCancel = cancelFunc
	config, err := rs.ToV2Config(conf)
	if err != nil {
		log.Printf("配置有误：%v", err)
		rs.ServerStatus = pb.ServerStatus_FAILED
		return
	}
	server, err := core.New(config)
	if err != nil {
		log.Printf("创建服务失败：%v", err)
		rs.ServerStatus = pb.ServerStatus_FAILED
		return
	}
	err = server.Start()
	if err != nil {
		log.Printf("服务启动失败：%v", err)
		rs.ServerStatus = pb.ServerStatus_FAILED
		return
	}
	rs.V2flyServer = server
	rs.ServerStatus = pb.ServerStatus_STARTED
	nowVersion = conf.Version
	log.Println("启动成功")
}

// 当版本不一致时
func onVersionNotEq(ctx context.Context, rs *rocket.RS) {
	log.Println("版本更新")
	remote, cancelFunc := context.WithTimeout(ctx, time.Second*5)
	defer cancelFunc()
	config, err := rs.FetchConfig(remote)
	if err != nil {
		log.Printf("获取配置失败%v", err)
		return
	}
	go start(ctx, rs, config)
}

// 监听配置变更
func watchConfig(ctx context.Context, rs *rocket.RS) {
	for {
		log.Println("心跳检测")
		client, err := rs.GetClient()
		if err != nil {
			log.Println("获取客户端失败")
			time.Sleep(time.Second)
			continue
		}
		cnext, cancel := context.WithTimeout(ctx, time.Second*5)
		heartRes, err := client.SendHeart(cnext, &pb.HeartReq{
			Version:      nowVersion,
			Key:          rs.Key,
			ServerStatus: rs.ServerStatus,
		})
		if heartRes != nil && (rs.ServerStatus != pb.ServerStatus_STARTING) {
			version := heartRes.Version
			if version != nowVersion {
				onVersionNotEq(ctx, rs)
			}
		}
		if heartRes != nil {
			executes := heartRes.Executes
			rs.Execute(ctx, executes)
		}
		cancel()
		time.Sleep(time.Second * 5)
	}

}
func main() {
	launch()
}
