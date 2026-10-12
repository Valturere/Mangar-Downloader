package download

import (
	"errors"
	"golang.org/x/sys/windows"
	"regexp"
	"strings"
)

var httpErrorPattern = regexp.MustCompile(`(?i)\bHTTP(?: Error| error)?\s*[:=]?\s*(401|403|404|429|5\d\d)\b`)

const networkFailure = "Не удалось подключиться к серверу видео. Проверьте сеть, VPN или сетевую фильтрацию."

func filesystemFailure(err error) string {
	if errors.Is(err, windows.ERROR_DISK_FULL) || errors.Is(err, windows.ERROR_HANDLE_DISK_FULL) {
		return "Недостаточно места на диске"
	}
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return "Нет доступа к файлам. Проверьте права доступа."
	}
	if errors.Is(err, windows.ERROR_FILE_EXISTS) || errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return "Файл уже существует"
	}
	return ""
}

func httpFailureMessage(line string) string {
	if match := httpErrorPattern.FindStringSubmatch(line); len(match) > 1 {
		switch match[1] {
		case "401":
			return "Для этого видео требуется авторизация (HTTP 401)."
		case "403":
			return "Сервер отклонил запрос (HTTP 403)."
		case "404":
			return "Видео или ссылка больше недоступны (HTTP 404)."
		case "429":
			return "Сервер временно ограничил загрузки. Попробуйте позже."
		default:
			return "Сервер видео временно недоступен. Попробуйте позже."
		}
	}
	line = strings.ToLower(line)
	if strings.Contains(line, "timed out") || strings.Contains(line, "timeout") {
		return networkFailure
	}
	for _, text := range []string{"connection refused", "connection reset", "network is unreachable", "name or service not known", "getaddrinfo failed", "unable to resolve", "tls handshake", "temporary failure in name resolution", "remote end closed connection"} {
		if strings.Contains(line, text) {
			return networkFailure
		}
	}
	if strings.Contains(line, "no space left on device") || strings.Contains(line, "not enough space") || strings.Contains(line, "disk full") {
		return "Недостаточно места на диске"
	}
	if cookieFallbackEligible(line) {
		return "Для этого видео требуется авторизация"
	}
	return ""
}
