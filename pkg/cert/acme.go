package cert

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"

	"github.com/go-acme/lego/v4/registration"
)

// ACMEUser ACME用户实现
type ACMEUser struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

// GetEmail 获取邮箱
func (u *ACMEUser) GetEmail() string {
	return u.Email
}

// GetRegistration 获取注册信息
func (u *ACMEUser) GetRegistration() *registration.Resource {
	return u.Registration
}

// GetPrivateKey 获取私钥
func (u *ACMEUser) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

// NewACMEUser 创建ACME用户
func NewACMEUser(email string) (*ACMEUser, error) {
	// 生成ECDSA私钥
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("生成私钥失败: %w", err)
	}

	return &ACMEUser{
		Email: email,
		key:   privateKey,
	}, nil
}
