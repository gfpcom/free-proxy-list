package internal

import (
	"fmt"
	"testing"
	"github.com/stretchr/testify/require"
)

func TestParser(t *testing.T) {
	u := "vless://Telegram-EXPRESSVPN_420@expressvpn_420.fast.hosting-ip.com:80/?type=ws&encryption=none&host=V2RAY_420.nettisbdaak.net&path=%2F%40EXPRESSVPN_420------%40EXPRESSVPN_420------%40EXPRESSVPN_420------%40EXPRESSVPN_420------%40EXPRESSVPN_420------%40EXPRESSVPN_420------%40EXPRESSVPN_420------%40EXPRESSVPN_420------%40EXPRESSVPN_420%3Fed%3D2048#%F0%9F%91%89%F0%9F%86%94%20%40v2ray_configs_pool%F0%9F%93%A1%F0%9F%87%BA%F0%9F%87%B8United%20States"

	proxy, err := ParseProxyURL("vless", u)

	require.NoError(t, err)
	fmt.Println(proxy)
}
