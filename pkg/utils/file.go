package utils

import (
	"fmt"
	"os"
	"path/filepath"
)

func includeFile(path string, match []string) bool {
	for _, m := range match {
		file := filepath.Base(path)
		fmt.Printf("path base: %s match: %s\n", file, m)
		if matched, err := filepath.Match(m, file); err == nil && matched {
			return true
		}
	}
	return false
}

func ReadFiles(source string, match []string) ([]string, error) {
	filePaths := make([]string, 0)

	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if includeFile(filepath.Ext(info.Name()), match) {
			filePaths = append(filePaths, path)
		} else {
			// check for exact file match
			if includeFile(info.Name(), match) {
				filePaths = append(filePaths, path)
			}
		}
		return nil
	})
	if err != nil {
		return filePaths, err
	}

	return filePaths, nil
}
