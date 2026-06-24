package notifier

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/yuin/goldmark"
)

// SMTPConfig 是 Channel.Config 解密后对应的 SMTP 参数 schema。
type SMTPConfig struct {
	Host         string   `json:"host"`
	Port         int      `json:"port"`
	Username     string   `json:"username"`
	Password     string   `json:"password_enc"` // 落库 _enc 后缀，传入时已是明文
	FromAddr     string   `json:"from_addr"`
	FromName     string   `json:"from_name"`
	To           []string `json:"to"`
	UseStartTLS  bool     `json:"use_starttls"`
	SecurityMode string   `json:"security_mode"`
}

type smtpNotifier struct{}

func (n *smtpNotifier) Type() string { return "smtp" }

func (n *smtpNotifier) Send(ctx context.Context, configJSON []byte, msg Message) error {
	var cfg SMTPConfig
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return Permanent(fmt.Errorf("解析 SMTP 配置失败: %w", err))
	}
	log.Printf("[smtp] 开始发送 subject=%q to=%v body_len=%d format=%s", msg.Subject, cfg.To, len(msg.Body), msg.Format)
	if cfg.Host == "" || cfg.Port == 0 || cfg.FromAddr == "" || len(cfg.To) == 0 {
		return Permanent(fmt.Errorf("SMTP 配置缺少必填字段"))
	}

	// 把同步 SMTP 调用包到 goroutine 里，以便响应 ctx 取消
	errCh := make(chan error, 1)
	go func() {
		errCh <- n.doSend(cfg, msg)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func (n *smtpNotifier) doSend(cfg SMTPConfig, msg Message) error {
	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	tlsCfg := &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}
	mode, err := resolveSMTPSecurityMode(cfg)
	if err != nil {
		return Permanent(err)
	}

	var conn net.Conn
	if mode == "implicit_tls" {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
		if err != nil {
			return fmt.Errorf("连接 SMTPS 服务器失败: %w", err)
		}
	} else {
		conn, err = dialer.Dial("tcp", addr)
		if err != nil {
			if cfg.Port == 465 && mode == "plain" {
				return fmt.Errorf("连接 SMTP 服务器失败: %w；465 端口通常需要直连 TLS，请将安全模式改为“直连 TLS（SMTPS）”或改用 587 + STARTTLS", err)
			}
			return fmt.Errorf("连接 SMTP 服务器失败: %w", err)
		}
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))

	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		if mode == "implicit_tls" {
			return fmt.Errorf("初始化 SMTPS 客户端失败: %w", err)
		}
		return fmt.Errorf("初始化 SMTP 客户端失败: %w", err)
	}
	defer client.Close()

	if mode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return Permanent(fmt.Errorf("SMTP 服务器不支持 STARTTLS，请改用直连 TLS（SMTPS）或明文 SMTP"))
		}
		if err := client.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("STARTTLS 失败: %w", err)
		}
	}

	if cfg.Username != "" {
		auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
		if err := client.Auth(auth); err != nil {
			// 鉴权失败不应重试
			return Permanent(fmt.Errorf("SMTP 鉴权失败: %w", err))
		}
	}

	if err := client.Mail(cfg.FromAddr); err != nil {
		return fmt.Errorf("MAIL FROM 失败: %w", err)
	}
	for _, to := range cfg.To {
		if err := client.Rcpt(to); err != nil {
			return Permanent(fmt.Errorf("RCPT TO %s 失败: %w", to, err))
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA 命令失败: %w", err)
	}
	if _, err := w.Write([]byte(buildMIME(cfg, msg))); err != nil {
		return fmt.Errorf("写邮件正文失败: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("关闭邮件流失败: %w", err)
	}

	return client.Quit()
}

// buildMIME 构造一个最小可用的 MIME 邮件文本：
// UTF-8 + Subject base64 编码，避免中文乱码。
func resolveSMTPSecurityMode(cfg SMTPConfig) (string, error) {
	switch cfg.SecurityMode {
	case "", "plain", "starttls", "implicit_tls":
	default:
		return "", fmt.Errorf("未知 SMTP 安全模式: %s", cfg.SecurityMode)
	}
	if cfg.SecurityMode != "" {
		return cfg.SecurityMode, nil
	}
	if cfg.UseStartTLS {
		return "starttls", nil
	}
	if cfg.Port == 465 {
		return "implicit_tls", nil
	}
	return "plain", nil
}

func buildMIME(cfg SMTPConfig, msg Message) string {
	from := cfg.FromAddr
	if cfg.FromName != "" {
		from = fmt.Sprintf("=?UTF-8?B?%s?= <%s>", base64Encode(cfg.FromName), cfg.FromAddr)
	}
	subjectEncoded := fmt.Sprintf("=?UTF-8?B?%s?=", base64Encode(msg.Subject))

	var contentType string
	var body string
	switch msg.Format {
	case "html":
		contentType = "text/html; charset=UTF-8"
		body = msg.Body
	case "markdown":
		contentType = "text/html; charset=UTF-8"
		body = renderMarkdownToHTML(msg.Body)
	default: // "text" 或未知
		contentType = "text/plain; charset=UTF-8"
		body = msg.Body
	}

	return strings.Join([]string{
		"From: " + from,
		"To: " + strings.Join(cfg.To, ", "),
		"Subject: " + subjectEncoded,
		"MIME-Version: 1.0",
		"Content-Type: " + contentType,
		"Content-Transfer-Encoding: 8bit",
		"",
		body,
	}, "\r\n")
}

var mdRenderer = goldmark.New()

func renderMarkdownToHTML(src string) string {
	var buf bytes.Buffer
	if err := mdRenderer.Convert([]byte(src), &buf); err != nil {
		return src // 降级返回原文
	}
	return buf.String()
}
