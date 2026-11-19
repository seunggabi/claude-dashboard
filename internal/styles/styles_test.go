package styles

import "testing"

func TestSessionColor_isStableAndSpread(t *testing.T) {
	if SessionColor("cd-api") != SessionColor("cd-api") {
		t.Fatal("same name gave different colors")
	}

	seen := map[string]bool{}
	for _, name := range []string{"cd-api-server", "cd-web-app", "cd-infra", "cd-docs", "cd-blog"} {
		seen[SessionColor(name)] = true
	}
	if len(seen) < 2 {
		t.Errorf("five names share %d color(s), want at least 2", len(seen))
	}
}
