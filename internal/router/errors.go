package router

func IsRetryableStatus(status int) bool {
	switch status {
	case 408, 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}
