package assets

import (
	"net/url"
	"os"
	"strings"
	"time"
)

// Match VidBee's GitHub release fallbacks. Each candidate still passes through
// the same temporary-file, complete-download and archive validation steps.
func githubDownloadURLs(raw string, preferChina bool) []string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return []string{raw}
	}
	host := parsed.Hostname()
	if host != "github.com" && host != "api.github.com" && !strings.HasSuffix(host, ".githubusercontent.com") {
		return []string{raw}
	}
	mirrors := []string{"https://ghfast.top/" + raw, "https://gh-proxy.com/" + raw}
	if preferChina {
		return append(mirrors, raw)
	}
	return append([]string{raw}, mirrors...)
}

func preferChinaMirrors() bool {
	switch os.Getenv("DAYBEE_DOWNLOAD_MIRROR") {
	case "cn":
		return true
	case "global":
		return false
	}
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if locale := os.Getenv(key); locale != "" {
			if strings.HasPrefix(strings.ToLower(strings.ReplaceAll(locale, "-", "_")), "zh_cn") {
				return true
			}
			break
		}
	}
	zone := os.Getenv("TZ")
	if zone == "Asia/Shanghai" || zone == "Asia/Chongqing" || zone == "PRC" {
		return true
	}
	name, offset := time.Now().Zone()
	return name == "CST" && offset == 8*60*60
}
