package cert

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nspass/nspass-agent/pkg/interfaces"
	"github.com/nspass/nspass-agent/pkg/logging"
	"github.com/sirupsen/logrus"
)

// CertificateInfo 证书信息
type CertificateInfo struct {
	Domain       string    `json:"domain"`        // 域名
	CertPath     string    `json:"cert_path"`     // 证书文件路径
	KeyPath      string    `json:"key_path"`      // 私钥文件路径
	IssuedAt     time.Time `json:"issued_at"`     // 签发时间
	ExpiresAt    time.Time `json:"expires_at"`    // 过期时间
	Issuer       string    `json:"issuer"`        // 签发机构
	SerialNumber string    `json:"serial_number"` // 序列号
}

// CertificateStore 证书存储接口
type CertificateStore interface {
	// SaveCertificate 保存证书
	SaveCertificate(domain string, certPEM, keyPEM []byte) (*CertificateInfo, error)

	// GetCertificate 获取证书信息
	GetCertificate(domain string) (*CertificateInfo, error)

	// ListCertificates 列出所有证书
	ListCertificates() ([]*CertificateInfo, error)

	// DeleteCertificate 删除证书
	DeleteCertificate(domain string) error

	// IsCertificateExpiring 检查证书是否即将过期
	IsCertificateExpiring(domain string, threshold time.Duration) (bool, error)
}

// FileCertificateStore 基于文件系统的证书存储
type FileCertificateStore struct {
	basePath string
	logger   interfaces.Logger
}

// NewFileCertificateStore 创建文件证书存储
func NewFileCertificateStore(basePath string) (*FileCertificateStore, error) {
	// 确保基础目录存在
	if err := os.MkdirAll(basePath, 0700); err != nil {
		return nil, fmt.Errorf("创建证书存储目录失败: %w", err)
	}

	return &FileCertificateStore{
		basePath: basePath,
		logger:   logging.GetComponentLogger("cert-store"),
	}, nil
}

// SaveCertificate 保存证书
func (s *FileCertificateStore) SaveCertificate(domain string, certPEM, keyPEM []byte) (*CertificateInfo, error) {
	s.logger.WithField("domain", domain).Info("保存证书")

	// 解析证书以获取信息
	certInfo, err := s.parseCertificate(certPEM)
	if err != nil {
		return nil, fmt.Errorf("解析证书失败: %w", err)
	}

	// 创建域名目录
	domainDir := filepath.Join(s.basePath, domain)
	if err := os.MkdirAll(domainDir, 0700); err != nil {
		return nil, fmt.Errorf("创建域名目录失败: %w", err)
	}

	// 保存证书文件
	certPath := filepath.Join(domainDir, "cert.pem")
	if err := os.WriteFile(certPath, certPEM, 0600); err != nil {
		return nil, fmt.Errorf("保存证书文件失败: %w", err)
	}

	// 保存私钥文件
	keyPath := filepath.Join(domainDir, "key.pem")
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return nil, fmt.Errorf("保存私钥文件失败: %w", err)
	}

	// 创建证书信息
	info := &CertificateInfo{
		Domain:       domain,
		CertPath:     certPath,
		KeyPath:      keyPath,
		IssuedAt:     certInfo.NotBefore,
		ExpiresAt:    certInfo.NotAfter,
		Issuer:       certInfo.Issuer.String(),
		SerialNumber: certInfo.SerialNumber.String(),
	}

	// 保存证书信息文件
	infoPath := filepath.Join(domainDir, "info.json")
	infoData, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("序列化证书信息失败: %w", err)
	}

	if err := os.WriteFile(infoPath, infoData, 0600); err != nil {
		return nil, fmt.Errorf("保存证书信息失败: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"domain":     domain,
		"expires_at": info.ExpiresAt,
		"issuer":     info.Issuer,
	}).Info("证书保存成功")

	return info, nil
}

// GetCertificate 获取证书信息
func (s *FileCertificateStore) GetCertificate(domain string) (*CertificateInfo, error) {
	infoPath := filepath.Join(s.basePath, domain, "info.json")

	data, err := os.ReadFile(infoPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // 证书不存在
		}
		return nil, fmt.Errorf("读取证书信息失败: %w", err)
	}

	var info CertificateInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("解析证书信息失败: %w", err)
	}

	return &info, nil
}

// ListCertificates 列出所有证书
func (s *FileCertificateStore) ListCertificates() ([]*CertificateInfo, error) {
	var certificates []*CertificateInfo

	entries, err := os.ReadDir(s.basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return certificates, nil
		}
		return nil, fmt.Errorf("读取证书目录失败: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		domain := entry.Name()
		info, err := s.GetCertificate(domain)
		if err != nil {
			s.logger.WithError(err).WithField("domain", domain).Warn("获取证书信息失败")
			continue
		}

		if info != nil {
			certificates = append(certificates, info)
		}
	}

	return certificates, nil
}

// DeleteCertificate 删除证书
func (s *FileCertificateStore) DeleteCertificate(domain string) error {
	s.logger.WithField("domain", domain).Info("删除证书")

	domainDir := filepath.Join(s.basePath, domain)
	if err := os.RemoveAll(domainDir); err != nil {
		return fmt.Errorf("删除证书目录失败: %w", err)
	}

	s.logger.WithField("domain", domain).Info("证书删除成功")
	return nil
}

// IsCertificateExpiring 检查证书是否即将过期
func (s *FileCertificateStore) IsCertificateExpiring(domain string, threshold time.Duration) (bool, error) {
	info, err := s.GetCertificate(domain)
	if err != nil {
		return false, fmt.Errorf("获取证书信息失败: %w", err)
	}

	if info == nil {
		return true, nil // 证书不存在，需要申请
	}

	// 检查是否即将过期
	timeUntilExpiry := time.Until(info.ExpiresAt)
	return timeUntilExpiry <= threshold, nil
}

// parseCertificate 解析证书PEM数据
func (s *FileCertificateStore) parseCertificate(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("无法解码PEM数据")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("解析X.509证书失败: %w", err)
	}

	return cert, nil
}
