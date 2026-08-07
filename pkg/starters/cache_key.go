package starters

// CacheKey returns the dedup/cache key for a starters request.
// productPath is preferred when present; linkText is the migration fallback.
func CacheKey(account, productPath, linkText string) string {
	if productPath != "" {
		return account + ":" + productPath
	}
	return account + ":" + linkText
}
