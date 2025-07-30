package utils

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// IPInfo 包含IP地址信息
type IPInfo struct {
	IPv4Address   string   `json:"ipv4_address"`
	IPv6Addresses []string `json:"ipv6_addresses"`
}

// IPInfoCollector IP信息收集器
type IPInfoCollector struct {
	httpClient *http.Client
}

// NewIPInfoCollector 创建IP信息收集器
func NewIPInfoCollector() *IPInfoCollector {
	return &IPInfoCollector{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetIPInfo 获取完整的IP信息
func (c *IPInfoCollector) GetIPInfo(ctx context.Context) (*IPInfo, error) {
	ipInfo := &IPInfo{}

	// 获取IPv4地址
	ipv4, err := c.GetPublicIPv4(ctx)
	if err != nil {
		// IPv4获取失败不应该阻止IPv6获取
		fmt.Printf("获取IPv4地址失败: %v\n", err)
	} else {
		ipInfo.IPv4Address = ipv4
	}

	// 获取IPv6地址
	ipv6Addresses, err := c.GetIPv6Addresses()
	if err != nil {
		// IPv6获取失败不应该阻止IPv4获取
		fmt.Printf("获取IPv6地址失败: %v\n", err)
	} else {
		ipInfo.IPv6Addresses = ipv6Addresses
	}

	return ipInfo, nil
}

// GetPublicIPv4 通过cip.cc获取公网IPv4地址
func (c *IPInfoCollector) GetPublicIPv4(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://cip.cc", nil)
	if err != nil {
		return "", fmt.Errorf("创建HTTP请求失败: %w", err)
	}

	// 设置User-Agent
	req.Header.Set("User-Agent", "nspass-agent/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP请求失败，状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	// 解析cip.cc的响应
	// cip.cc返回格式类似：
	// IP	: 1.2.3.4
	// 地址	: 中国  北京
	// 运营商	: 联通
	lines := strings.Split(string(body), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "IP") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				ip := strings.TrimSpace(parts[1])
				// 验证IP地址格式
				if net.ParseIP(ip) != nil {
					return ip, nil
				}
			}
		}
	}

	return "", fmt.Errorf("无法从cip.cc响应中解析IP地址: %s", string(body))
}

// GetIPv6Addresses 获取系统的IPv6地址
func (c *IPInfoCollector) GetIPv6Addresses() ([]string, error) {
	var ipv6Addresses []string

	// 获取所有网络接口
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("获取网络接口失败: %w", err)
	}

	for _, iface := range interfaces {
		// 跳过回环接口和未启用的接口
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		// 获取接口的地址
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// 检查是否为IPv6地址且不是链路本地地址
			if ip != nil && ip.To4() == nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
				// 过滤掉临时地址和私有地址，只保留全局单播地址
				if ip.IsGlobalUnicast() {
					ipv6Addresses = append(ipv6Addresses, ip.String())
				}
			}
		}
	}

	return ipv6Addresses, nil
}
