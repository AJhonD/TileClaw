package mail

import (
	"fmt"
	"net/smtp"
	"sync"
	"tileclaw/internal/config"
	"tileclaw/internal/log"
	"time"
)

var (
	errorCount  int
	errorMu     sync.Mutex
	lastAlert   time.Time
	windowStart time.Time
)

func init() {
	windowStart = time.Now()
}

func SendMail(subject, body string) {
	if !config.Mail.EmailConfig.Enabled {
		return
	}
	addr := fmt.Sprintf("%s:%d", config.Mail.EmailConfig.Host, config.Mail.EmailConfig.Port)
	auth := smtp.PlainAuth("", config.Mail.EmailConfig.Username, config.Mail.EmailConfig.Password, config.Mail.EmailConfig.Host)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		config.Mail.EmailConfig.Username, config.Mail.EmailConfig.To, subject, body)

	err := smtp.SendMail(addr, auth, config.Mail.EmailConfig.Username, []string{config.Mail.EmailConfig.To}, []byte(msg))
	if err != nil {
		log.SysLog.Errorf("send email failed: %s", err)
	} else {
		log.SysLog.Infof("email sent: %s", subject)
	}
}

// CountError 累计错误并检查是否触发告警
func CountError() {
	if !config.Mail.EmailConfig.Enabled {
		return
	}
	errorMu.Lock()
	defer errorMu.Unlock()

	// 窗口过期，重置
	if time.Since(windowStart) > time.Duration(config.Mail.AlertConfig.WindowMinutes)*time.Minute {
		errorCount = 0
		windowStart = time.Now()
	}
	errorCount++

	if errorCount >= config.Mail.AlertConfig.ErrorThreshold && time.Since(lastAlert) > time.Duration(config.Mail.AlertConfig.CooldownMinutes)*time.Minute {
		lastAlert = time.Now()
		subject := fmt.Sprintf("[%s] Error Alert: %d errors in %d minutes",
			config.Mail.EmailConfig.TaskName, errorCount, config.Mail.AlertConfig.WindowMinutes)
		body := fmt.Sprintf("TileClaw download task \"%s\" has encountered %d errors within %d minutes.\n\nPlease check logs/tileclaw.log for details.",
			config.Mail.EmailConfig.TaskName, errorCount, config.Mail.AlertConfig.WindowMinutes)
		SendMail(subject, body)
	}
}

// NotifyComplete 下载完成通知
func NotifyComplete(completed, total int64, elapsed time.Duration) {
	if !config.Mail.EmailConfig.Enabled {
		return
	}
	pct := float64(completed) / float64(total) * 100
	subject := fmt.Sprintf("[%s] Download Complete: %.1f%%", config.Mail.EmailConfig.TaskName, pct)
	body := fmt.Sprintf(`TileClaw download task "%s" finished.

Result: %d / %d tiles (%.1f%%)
Elapsed: %s
Output: %s.mbtiles

This is an automated notification from TileClaw.`,
		config.Mail.EmailConfig.TaskName, completed, total, pct, elapsed.Truncate(time.Second), config.Mail.EmailConfig.TaskName)
	SendMail(subject, body)
}

// NotifyPanic panic 告警
func NotifyPanic(detail string) {
	if !config.Mail.EmailConfig.Enabled {
		return
	}
	subject := fmt.Sprintf("[%s] PANIC: program crashed", config.Mail.EmailConfig.TaskName)
	body := fmt.Sprintf("TileClaw download task \"%s\" crashed with panic:\n\n%s\n\nCheck logs/tileclaw.log for stack trace.",
		config.Mail.EmailConfig.TaskName, detail)
	SendMail(subject, body)
}
