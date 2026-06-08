package notifier

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestResolveSMTPSecurityMode(t *testing.T) {
	cases := []struct {
		name string
		cfg  SMTPConfig
		want string
	}{
		{name: "显式直连 TLS", cfg: SMTPConfig{SecurityMode: "implicit_tls"}, want: "implicit_tls"},
		{name: "显式 STARTTLS", cfg: SMTPConfig{SecurityMode: "starttls"}, want: "starttls"},
		{name: "显式明文", cfg: SMTPConfig{SecurityMode: "plain"}, want: "plain"},
		{name: "兼容旧 STARTTLS", cfg: SMTPConfig{UseStartTLS: true, Port: 587}, want: "starttls"},
		{name: "兼容旧 465", cfg: SMTPConfig{Port: 465}, want: "implicit_tls"},
		{name: "默认明文", cfg: SMTPConfig{Port: 25}, want: "plain"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveSMTPSecurityMode(tc.cfg)
			if err != nil {
				t.Fatalf("resolveSMTPSecurityMode 失败: %v", err)
			}
			if got != tc.want {
				t.Fatalf("want %s, got %s", tc.want, got)
			}
		})
	}
}

func TestResolveSMTPSecurityModeRejectsUnknownMode(t *testing.T) {
	_, err := resolveSMTPSecurityMode(SMTPConfig{SecurityMode: "bad-mode"})
	if err == nil {
		t.Fatal("未知安全模式应报错")
	}
}

func TestSMTPPlain465DialErrorHintsImplicitTLSMismatch(t *testing.T) {
	cfg, err := json.Marshal(SMTPConfig{
		Host:         "127.0.0.1",
		Port:         465,
		FromAddr:     "sender@example.com",
		To:           []string{"receiver@example.com"},
		SecurityMode: "plain",
	})
	if err != nil {
		t.Fatalf("构造配置失败: %v", err)
	}

	err = (&smtpNotifier{}).Send(context.Background(), cfg, Message{Subject: "s", Body: "b"})
	if err == nil {
		t.Fatal("期望连接失败")
	}
	if !strings.Contains(err.Error(), "465 端口通常需要直连 TLS") {
		t.Fatalf("错误提示未包含协议修复建议: %v", err)
	}
}

func TestSMTPMissingRequiredFieldsIsPermanent(t *testing.T) {
	cfg, err := json.Marshal(SMTPConfig{Host: "smtp.example.com"})
	if err != nil {
		t.Fatalf("构造配置失败: %v", err)
	}

	err = (&smtpNotifier{}).Send(context.Background(), cfg, Message{})
	if err == nil {
		t.Fatal("期望缺少必填字段时报错")
	}
	if !IsPermanent(err) {
		t.Fatalf("缺少必填字段应为永久错误，实际: %v", err)
	}
}
