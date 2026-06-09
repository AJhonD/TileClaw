package main

import (
	"fmt"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
)

type mailConfig struct {
	enabled  bool
	host     string
	port     int
	username string
	password string
	to       string
	taskName string
	// alert settings
	errorThreshold  int
	cooldownMinutes int
	windowMinutes   int
}

var (
	mailCfg     mailConfig
	errorCount  int
	errorMu     sync.Mutex
	lastAlert   time.Time
	windowStart time.Time
)

func loadMailConfig() {
	v := viper.New()
	v.SetConfigFile("mail.toml")
	v.SetConfigType("toml")
	v.SetEnvPrefix("TILECLAW")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		sysLog.Warnf("read mail.toml failed: %s, using environment/default mail config", err)
	}
	mailCfg = mailConfig{
		enabled:         v.GetBool("email.enabled"),
		host:            v.GetString("email.smtp_host"),
		port:            v.GetInt("email.smtp_port"),
		username:        v.GetString("email.username"),
		password:        v.GetString("email.password"),
		to:              v.GetString("email.to"),
		taskName:        v.GetString("email.task_name"),
		errorThreshold:  v.GetInt("alert.error_threshold"),
		cooldownMinutes: v.GetInt("alert.cooldown_minutes"),
		windowMinutes:   v.GetInt("alert.window_minutes"),
	}
	if mailCfg.errorThreshold == 0 {
		mailCfg.errorThreshold = 50
	}
	if mailCfg.cooldownMinutes == 0 {
		mailCfg.cooldownMinutes = 30
	}
	if mailCfg.windowMinutes == 0 {
		mailCfg.windowMinutes = 10
	}
	if mailCfg.enabled && (mailCfg.host == "" || mailCfg.port == 0 || mailCfg.username == "" || mailCfg.password == "" || mailCfg.to == "") {
		sysLog.Warnf("email enabled but mail config is incomplete, email disabled")
		mailCfg.enabled = false
	}
	windowStart = time.Now()
	sysLog.Infof("mail config loaded, enabled=%v, to=%s", mailCfg.enabled, mailCfg.to)
}

func sendMail(subject, body string) {
	if !mailCfg.enabled {
		return
	}
	addr := fmt.Sprintf("%s:%d", mailCfg.host, mailCfg.port)
	auth := smtp.PlainAuth("", mailCfg.username, mailCfg.password, mailCfg.host)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		mailCfg.username, mailCfg.to, subject, body)

	err := smtp.SendMail(addr, auth, mailCfg.username, []string{mailCfg.to}, []byte(msg))
	if err != nil {
		sysLog.Errorf("send email failed: %s", err)
	} else {
		sysLog.Infof("email sent: %s", subject)
	}
}

// 累计错误并检查是否触发告警
func countError() {
	if !mailCfg.enabled {
		return
	}
	errorMu.Lock()
	defer errorMu.Unlock()

	// 窗口过期，重置
	if time.Since(windowStart) > time.Duration(mailCfg.windowMinutes)*time.Minute {
		errorCount = 0
		windowStart = time.Now()
	}
	errorCount++

	if errorCount >= mailCfg.errorThreshold && time.Since(lastAlert) > time.Duration(mailCfg.cooldownMinutes)*time.Minute {
		lastAlert = time.Now()
		subject := fmt.Sprintf("[%s] Error Alert: %d errors in %d minutes",
			mailCfg.taskName, errorCount, mailCfg.windowMinutes)
		body := fmt.Sprintf("TileClaw download task \"%s\" has encountered %d errors within %d minutes.\n\nPlease check tileclaw.log for details.",
			mailCfg.taskName, errorCount, mailCfg.windowMinutes)
		sendMail(subject, body)
	}
}

// 下载完成通知
func notifyComplete(completed, total int64, elapsed time.Duration) {
	if !mailCfg.enabled {
		return
	}
	pct := float64(completed) / float64(total) * 100
	subject := fmt.Sprintf("[%s] Download Complete: %.1f%%", mailCfg.taskName, pct)
	body := fmt.Sprintf(`TileClaw download task "%s" finished.

Result: %d / %d tiles (%.1f%%)
Elapsed: %s
Output: %s.mbtiles

This is an automated notification from TileClaw.`,
		mailCfg.taskName, completed, total, pct, elapsed.Truncate(time.Second), mailCfg.taskName)
	sendMail(subject, body)
}

// panic 告警
func notifyPanic(detail string) {
	if !mailCfg.enabled {
		return
	}
	subject := fmt.Sprintf("[%s] PANIC: program crashed", mailCfg.taskName)
	body := fmt.Sprintf("TileClaw download task \"%s\" crashed with panic:\n\n%s\n\nCheck tileclaw.log for stack trace.",
		mailCfg.taskName, detail)
	sendMail(subject, body)
}
