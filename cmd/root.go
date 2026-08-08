package cmd

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/PastureStack/catalog-service/manager"
	"github.com/PastureStack/catalog-service/model"
	"github.com/PastureStack/catalog-service/service"
	"github.com/go-sql-driver/mysql"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	refreshInterval int
	port            int
	cacheRoot       string
	configFile      string
	validateOnly    bool
	sqlite          bool
	migrateDb       bool
	trackCompat     bool
	debug           bool
	version         bool
	locale          string
	VERSION         string
)

var RootCmd = &cobra.Command{
	Use: "catalog-service",
	Run: run,
}

func init() {
	viper.SetEnvPrefix("catalog_service")
	viper.AutomaticEnv()

	RootCmd.PersistentFlags().IntVar(&refreshInterval, "refresh-interval", 60, "")
	RootCmd.PersistentFlags().IntVarP(&port, "port", "p", 8088, "")
	RootCmd.PersistentFlags().StringVar(&cacheRoot, "cache", "./cache", "")
	RootCmd.PersistentFlags().StringVar(&configFile, "config", "./repo.json", "")
	RootCmd.PersistentFlags().BoolVar(&validateOnly, "validate", false, "")
	RootCmd.PersistentFlags().BoolVar(&sqlite, "sqlite", false, "")
	RootCmd.PersistentFlags().BoolVar(&migrateDb, "migrate-db", false, "")
	RootCmd.PersistentFlags().BoolVar(&trackCompat, "track", false, "Deprecated compatibility flag; no installation identifier is read or transmitted")
	RootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "")
	RootCmd.PersistentFlags().BoolVarP(&version, "version", "v", false, "")
	localeDefault := os.Getenv("PASTURESTACK_LOCALE")
	if localeDefault == "" {
		localeDefault = "en-US"
	}
	RootCmd.PersistentFlags().StringVar(&locale, "locale", localeDefault, "Operator message locale: en-US or zh-TW")

	RootCmd.PersistentFlags().String("mysql-user", "", "")
	viper.BindPFlag("mysql_user", RootCmd.PersistentFlags().Lookup("mysql-user"))

	RootCmd.PersistentFlags().String("mysql-password", "", "")
	viper.BindPFlag("mysql_password", RootCmd.PersistentFlags().Lookup("mysql-password"))

	RootCmd.PersistentFlags().String("mysql-address", "", "")
	viper.BindPFlag("mysql_address", RootCmd.PersistentFlags().Lookup("mysql-address"))

	RootCmd.PersistentFlags().String("mysql-dbname", "", "")
	viper.BindPFlag("mysql_dbname", RootCmd.PersistentFlags().Lookup("mysql-dbname"))

	RootCmd.PersistentFlags().String("mysql-params", "", "")
	viper.BindPFlag("mysql_params", RootCmd.PersistentFlags().Lookup("mysql-params"))
}

func run(cmd *cobra.Command, args []string) {
	if locale != "en-US" && locale != "zh-TW" {
		log.Fatalf("unsupported locale %q; use en-US or zh-TW", locale)
	}
	if version {
		fmt.Println(VERSION)
		return
	}
	if debug {
		log.SetLevel(log.DebugLevel)
	}

	var db *gorm.DB
	var err error
	if sqlite {
		if !sqliteAvailable() {
			log.Fatal("SQLite support is not available in this binary")
		}
		db, err = gorm.Open("sqlite3", "local.db")
		if err != nil {
			log.Fatal(err)
		}
		db.Exec("PRAGMA foreign_keys = ON")
		migrateDb = true
	} else {
		user := viper.GetString("mysql_user")
		password := viper.GetString("mysql_password")
		address := viper.GetString("mysql_address")
		dbname := viper.GetString("mysql_dbname")
		params := viper.GetString("mysql_params")

		db, err = gorm.Open("mysql", formatDSN(user, password, address, dbname, params))
		if err != nil {
			log.Fatal(err)
		}
	}
	defer db.Close()

	db.SingularTable(true)
	gorm.DefaultTableNameHandler = func(db *gorm.DB, defaultTableName string) string {
		defaultTableName = strings.TrimSuffix(defaultTableName, "_model")
		if defaultTableName == "catalog" {
			return defaultTableName
		}
		if defaultTableName == "template_label" {
			defaultTableName = strings.TrimPrefix(defaultTableName, "template_")
		}
		return "catalog_" + defaultTableName
	}

	if migrateDb {
		log.Info("Migrating DB")
		db.AutoMigrate(&model.CatalogModel{})

		db.AutoMigrate(&model.TemplateModel{})
		db.AutoMigrate(&model.CategoryModel{})
		db.AutoMigrate(&model.TemplateCategoryModel{})
		db.AutoMigrate(&model.TemplateLabelModel{})

		db.AutoMigrate(&model.VersionModel{})
		db.AutoMigrate(&model.FileModel{})
		db.AutoMigrate(&model.VersionLabelModel{})
	}

	m, err := manager.NewManager(cacheRoot, configFile, validateOnly, db)
	if err != nil {
		log.Fatal("Invalid catalog source policy")
	}
	if validateOnly {
		if err := m.RefreshAll(true); err != nil {
			log.Fatal("Failed to validate catalog")
		}
		return
	}
	go autoRefresh(m, refreshInterval)

	log.Infof("%s (port %d, refresh interval %d seconds)", operatorMessage(locale, "start"), port, refreshInterval)

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), &service.MuxWrapper{
		IsReady: false,
		Router:  service.NewRouter(m, db),
	}))
}

func operatorMessage(locale, key string) string {
	messages := map[string]map[string]string{
		"en-US": {"start": "Starting PastureStack catalog service"},
		"zh-TW": {"start": "正在啟動 PastureStack 應用目錄服務"},
	}
	return messages[locale][key]
}

func formatDSN(user, password, address, dbname, params string) string {
	paramsMap := map[string]string{
		"parseTime": "true",
	}
	for _, param := range strings.Split(params, "&") {
		split := strings.SplitN(param, "=", 2)
		if len(split) > 1 {
			paramsMap[split[0]] = split[1]
		}
	}
	mysqlConfig := &mysql.Config{
		User:   user,
		Passwd: password,
		Net:    "tcp",
		Addr:   address,
		DBName: dbname,
		Params: paramsMap,
	}
	return mysqlConfig.FormatDSN()
}

func autoRefresh(m *manager.Manager, refreshInterval int) {
	var r = func(m *manager.Manager, update bool) {
		if err := m.RefreshAll(update); err != nil {
			if re, ok := err.(*manager.RepoRefreshError); ok {
				log.WithField("error_count", len(re.Errors)).Error("Catalog refresh failed")
			} else {
				log.Error("Catalog refresh failed")
			}
		}
	}
	// Refresh once without trying to update sources in case internet access isn't available
	r(m, false)

	r(m, true)
	if refreshInterval <= 0 {
		log.Warn("Automatic catalog refresh is disabled because the interval is not positive")
		return
	}
	ticker := time.NewTicker(time.Duration(refreshInterval) * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		log.Debugf("Performing automatic refresh of all catalogs (interval %d seconds)", refreshInterval)
		r(m, true)
	}
}
