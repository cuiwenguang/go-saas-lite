package migrations

import (
	"fmt"
	"io/fs"
	"regexp"
	"sort"
)

// createTablePattern 匹配迁移脚本里的建表语句。
var createTablePattern = regexp.MustCompile(`(?i)create\s+table\s+(?:if\s+not\s+exists\s+)?["` + "`" + `]?([a-zA-Z_][a-zA-Z0-9_]*)`)

// TableNames 解析指定目录下全部脚本创建的租户表名，去重并排序。
func TableNames(dir string) ([]string, error) {
	entries, err := fs.ReadDir(FS, dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir %s: %w", dir, err)
	}

	seen := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		content, readErr := fs.ReadFile(FS, dir+"/"+entry.Name())
		if readErr != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), readErr)
		}

		for _, match := range createTablePattern.FindAllStringSubmatch(string(content), -1) {
			seen[match[1]] = struct{}{}
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)

	return names, nil
}
