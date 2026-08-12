package protocol

import (
	"fmt"
	"net/url"
	"strconv"
)

func parseURLPort(u *url.URL, defaultPort int) (int, string, error) {
	return parsePort(u.Port(), defaultPort)
}

func parsePort(rawPort string, defaultPort int) (int, string, error) {
	if rawPort == "" {
		rawPort = strconv.Itoa(defaultPort)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil || port < 1 || port > 65535 {
		return 0, "", fmt.Errorf("无效端口: %s", rawPort)
	}
	return port, rawPort, nil
}
