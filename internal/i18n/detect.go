package i18n

// DetectSystemLang 返回 OS 映射后的生效语言：zh-CN | en | ja。
func DetectSystemLang() string {
	return MapOSLocale(DetectOSLocaleRaw())
}
