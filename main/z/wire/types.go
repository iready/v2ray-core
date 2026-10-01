package wire

import pb "github.com/v2fly/v2ray-core/v5/main/z/proto"

// ConfigPayload v2fly.agent.hello / v2fly.config.push 载荷。
type ConfigPayload struct {
	Ok      bool         `json:"ok"`
	Code    int32        `json:"code"`
	Msg     string       `json:"msg"`
	Version int32        `json:"version"`
	Config  []ConfigItem `json:"config"`
	ApiConf *ApiConf     `json:"api_conf"`
}

type ConfigItem struct {
	Key     string `json:"key"`
	Config  string `json:"config"`
	Version int32  `json:"version"`
}

type ApiConf struct {
	Port int32 `json:"port"`
}

type executeWire struct {
	OrderType int32    `json:"orderType"`
	FilePath  string   `json:"filePath"`
	Content   string   `json:"content"`
	ID        string   `json:"id"`
	BinInfo   string   `json:"binInfo"`
	Args      []string `json:"args"`
}

func (p *ConfigPayload) ToGetConfigRes() *pb.GetConfigRes {
	res := &pb.GetConfigRes{
		Version: p.Version,
		R:       &pb.R{Code: pb.Code(p.Code), Msg: p.Msg},
	}
	if p.Ok {
		res.R.Code = pb.Code_OK
	}
	for _, item := range p.Config {
		res.Config = append(res.Config, &pb.ConfigItem{
			Key:     item.Key,
			Config:  item.Config,
			Version: item.Version,
		})
	}
	if p.ApiConf != nil && p.ApiConf.Port > 0 {
		res.ApiConf = &pb.APIConf{Port: p.ApiConf.Port}
	}
	return res
}

func executesToProto(items []executeWire) []*pb.Execute {
	out := make([]*pb.Execute, 0, len(items))
	for _, e := range items {
		out = append(out, &pb.Execute{
			OrderType: pb.OrderType(e.OrderType),
			FilePath:  e.FilePath,
			Content:   e.Content,
			Id:        e.ID,
			BinInfo:   e.BinInfo,
			Args:      e.Args,
		})
	}
	return out
}
