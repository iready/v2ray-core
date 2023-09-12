package res

import (
	_ "embed"
	"github.com/v2fly/v2ray-core/v5/app/router/routercommon"
	"github.com/v2fly/v2ray-core/v5/infra/conf/geodata"
	"google.golang.org/protobuf/proto"
	"strings"
)

//go:embed geoip.dat
var GeoIP []byte

//go:embed geosite.dat
var GeoSite []byte

func loadIP(filename, country string) ([]*routercommon.CIDR, error) {
	var geoipList routercommon.GeoIPList
	if err := proto.Unmarshal(GeoIP, &geoipList); err != nil {
		return nil, err
	}

	for _, geoip := range geoipList.Entry {
		if strings.EqualFold(geoip.CountryCode, country) {
			return geoip.Cidr, nil
		}
	}

	return nil, nil
}

type standardLoader struct{}

func (d standardLoader) LoadSite(filename, list string) ([]*routercommon.Domain, error) {
	return loadSite(filename, list)
}

func loadSite(filename, list string) ([]*routercommon.Domain, error) {
	var geositeList routercommon.GeoSiteList
	if err := proto.Unmarshal(GeoSite, &geositeList); err != nil {
		return nil, err
	}

	for _, site := range geositeList.Entry {
		if strings.EqualFold(site.CountryCode, list) {
			return site.Domain, nil
		}
	}

	return nil, nil
}
func (d standardLoader) LoadIP(filename, country string) ([]*routercommon.CIDR, error) {
	return loadIP(filename, country)
}

func init() {
	geodata.RegisterGeoDataLoaderImplementationCreator("standard", func() geodata.LoaderImplementation {
		return standardLoader{}
	})
}
