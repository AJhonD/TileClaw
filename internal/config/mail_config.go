package config

import (
	"os"
	"path/filepath"
	"tileclaw/internal/log"

	"github.com/spf13/viper"
)

var Mail MailConfig

type MailConfig struct {
	EmailConfig EmailConfig `mapstructure:"email"`
	AlertConfig AlertConfig `mapstructure:"alert"`
}

type EmailConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Host     string `mapstructure:"smtp_host"`
	Port     int    `mapstructure:"smtp_port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	To       string `mapstructure:"to"`
	TaskName string `mapstructure:"task_name"`
}

type AlertConfig struct {
	ErrorThreshold  int `mapstructure:"error_threshold"`
	CooldownMinutes int `mapstructure:"cooldown_minutes"`
	WindowMinutes   int `mapstructure:"window_minutes"`
}

func newDefaultMailConfig() MailConfig {
	return MailConfig{
		EmailConfig: EmailConfig{
			Enabled: false,
		},
		AlertConfig: AlertConfig{
			ErrorThreshold:  50,
			CooldownMinutes: 30,
			WindowMinutes:   10,
		},
	}
}

func InitMailConfig(configDir string) {
	Mail = newDefaultMailConfig()

	mailFile := resolveMailConfigFile(configDir)
	if mailFile == "" {
		log.SysLog.Warnf("mail config not found, email disabled")
		return
	}

	mailV := viper.New()
	mailV.SetConfigFile(mailFile)
	mailV.SetConfigType("toml")
	mailV.AutomaticEnv()

	if err := mailV.ReadInConfig(); err != nil {
		log.SysLog.Fatal("read mail config failed: ", err)
	}
	if err := mailV.Unmarshal(&Mail); err != nil {
		log.SysLog.Fatal("parse mail config failed: ", err)
	}

	log.SysLog.Debugf("mail config loaded: %+v", Mail)
	if Mail.EmailConfig.Enabled && (Mail.EmailConfig.Host == "" || Mail.EmailConfig.Port == 0 || Mail.EmailConfig.Username == "" || Mail.EmailConfig.Password == "" || Mail.EmailConfig.To == "") {
		log.SysLog.Warnf("email enabled but mail config is incomplete, email disabled")
		Mail.EmailConfig.Enabled = false
	}
}

func resolveMailConfigFile(configDir string) string {
	candidates := []string{
		filepath.Join(configDir, "mail.toml"),
		filepath.Join("conf", "mail.toml"),
		"mail.toml",
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}
