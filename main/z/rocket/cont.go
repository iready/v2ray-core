package rocket

import (
	"context"
	core "github.com/v2fly/v2ray-core/v5"
	pb "github.com/v2fly/v2ray-core/v5/main/z/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"os"
	"os/exec"
	"time"
)

type RS struct {
	Address      string
	Key          string
	configClient pb.ConfigSyncClient
	ServerStatus pb.ServerStatus
	ServerCancel context.CancelFunc
	V2flyServer  *core.Instance
}

func (x *RS) GetClient() (pb.ConfigSyncClient, error) {
	if x.configClient != nil {
		return x.configClient, nil
	}
	dial, err := grpc.Dial(x.Address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(err)
		return nil, err
	}
	client := pb.NewConfigSyncClient(dial)
	x.configClient = client
	return x.configClient, nil
}

// FetchConfig 获取配置
func (x *RS) FetchConfig(c context.Context) (*pb.GetConfigRes, error) {
	client, err := x.GetClient()
	if err != nil {
		log.Fatal(err)
		return nil, err
	}
	ctx, cancelFunc := context.WithTimeout(c, time.Second)
	defer cancelFunc()
	r, err := client.GetConfig(ctx, &pb.GetConfigReq{
		Key: x.Key,
	})
	if err != nil {
		return nil, err
	}
	return r, nil
}

// LoopFetch 循环获取
func (x *RS) LoopFetch(c context.Context, waitSeconds int32) *pb.GetConfigRes {
	client, err := x.FetchConfig(c)
	for client == nil {
		log.Printf("%v秒后重新获取配置", waitSeconds)
		log.Printf("%v", err)
		time.Sleep(time.Second * 5)
		client, _ = x.FetchConfig(c)
	}
	return client
}
func (x *RS) ToV2Config(c *pb.GetConfigRes) (*core.Config, error) {
	remoteConfig := c.Config
	out := &core.Config{}
	out, err := core.LoadConfig(core.FormatJSON, []byte(remoteConfig))
	if err != nil {
		return nil, err
	}
	return out, nil
}
func (x *RS) UploadExecuteError(ctx context.Context, c *pb.Execute, uploadErr error) {
	client, err := x.GetClient()
	if err != nil {
		return
	}
	timeCtx, cancelFunc := context.WithTimeout(ctx, time.Second*5)
	defer cancelFunc()
	if uploadErr != nil {
		_, err = client.UploadExecuteInfo(timeCtx, &pb.UploadExecMsgReq{
			Id:  c.Id,
			Msg: uploadErr.Error(),
			Ok:  false})
	} else {
		_, err = client.UploadExecuteInfo(timeCtx, &pb.UploadExecMsgReq{
			Id:  c.Id,
			Msg: "执行成功",
			Ok:  true})
	}
	if err != nil {
		return
	}
}

// Execute 执行命令
func (x *RS) Execute(ctx context.Context, c []*pb.Execute) {
	for i, execute := range c {
		log.Printf("执行%v", i)
		if execute.OrderType == pb.OrderType_WRITEFILE {
			create, err := os.Create(execute.FilePath)
			if err != nil {
				go x.UploadExecuteError(ctx, execute, err)
				log.Printf("打开文件失败%v", err)
				break
			}
			defer create.Close()
			_, err = create.WriteString(execute.Content)
			if err != nil {
				go x.UploadExecuteError(ctx, execute, err)
				log.Printf("写入文件失败%v", err)
				break
			}
			go x.UploadExecuteError(ctx, execute, err)
		} else if execute.OrderType == pb.OrderType_EXECUTE {
			_, err := exec.Command(execute.BinInfo, execute.Args...).Output()

			if err != nil {
				go x.UploadExecuteError(ctx, execute, err)
				log.Printf("执行命令失败%v", err)
				break
			}
			go x.UploadExecuteError(ctx, execute, err)
		}
	}
}
