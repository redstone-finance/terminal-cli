package main

import "testing"

// embed.FS is an io/fs.FS, so it only understands '/'. filepath.Join would
// produce backslashes on Windows and no metadata file would ever be found.
func TestLoadConfigRulesUsesSlashPaths(t *testing.T) {
	if _, err := configFS.ReadFile(`metadata\trade\_2025_01_01.json`); err == nil {
		t.Fatal("embed.FS resolved a backslash path; this test no longer proves anything")
	}

	for _, dType := range []string{"trade", "derivative"} {
		rules, err := loadConfigRules(dType)
		if err != nil {
			t.Fatalf("loadConfigRules(%q): %v", dType, err)
		}
		if len(rules) == 0 {
			t.Errorf("loadConfigRules(%q) found no metadata", dType)
		}
		for _, r := range rules {
			if len(r.Config) == 0 {
				t.Errorf("%s rule for %s decoded to an empty config", dType, r.StartDate)
			}
		}
	}
}
