package tracking

import (
	"os"
	"time"

	platformclient "github.com/rancher/go-rancher/v2"
	log "github.com/sirupsen/logrus"
)

const (
	uuidSetting = "install.uuid"
	uuidPattern = "[[:xdigit:]]{8}-[[:xdigit:]]{4}-[[:xdigit:]]{4}-[[:xdigit:]]{4}-[[:xdigit:]]{12}"
)

var logger = log.WithFields(log.Fields{"service": "catalog"})

func LoadPlatformUUID() (string, error) {
	client, err := platformclient.NewRancherClient(&platformclient.ClientOpts{
		Url:       preferredEnvironment("CATALOG_SERVICE_PLATFORM_URL", "CATALOG_SERVICE_CATTLE_URL"),
		AccessKey: preferredEnvironment("CATALOG_SERVICE_PLATFORM_ACCESS_KEY", "CATALOG_SERVICE_CATTLE_ACCESS_KEY"),
		SecretKey: preferredEnvironment("CATALOG_SERVICE_PLATFORM_SECRET_KEY", "CATALOG_SERVICE_CATTLE_SECRET_KEY"),
		Timeout:   5 * time.Second,
	})

	uuid := ""
	if err != nil {
		return uuid, err
	}

	var setting *platformclient.Setting
	if setting, err = client.Setting.ById(uuidSetting); err != nil {
		logger.WithFields(log.Fields{
			"setting": "install.uuid",
			"error":   err.Error(),
		}).Warn("Failed to read setting")

	} else if setting == nil {
		logger.WithField("setting", "install.uuid").Warn("Setting is missing")
	} else if setting.Value == "" {
		logger.WithField("setting", "install.uuid").Warn("Setting is empty")
	} else {
		uuid = setting.Value
	}

	return uuid, nil
}

func preferredEnvironment(preferred, legacy string) string {
	if value := os.Getenv(preferred); value != "" {
		return value
	}
	return os.Getenv(legacy)
}
