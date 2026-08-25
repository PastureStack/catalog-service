package model

import "gorm.io/gorm"

type File struct {
	VersionId uint `gorm:"type:integer REFERENCES catalog_version(id) ON DELETE CASCADE"`

	Name     string `json:"name"`
	Contents string
}

type FileModel struct {
	Base
	File
}

func lookupFiles(db *gorm.DB, versionId uint) []File {
	var fileModels []FileModel
	db.Where("version_id = ?", versionId).Find(&fileModels)

	var files []File
	for _, fileModel := range fileModels {
		files = append(files, fileModel.File)
	}
	return files
}
