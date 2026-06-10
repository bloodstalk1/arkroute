package panel

import (
	"errors"
	"net/http"

	"github.com/bloodstalk1/arkroute/internal/config"
)

func httpStatusForSaveError(err error) int {
	if err == nil {
		return http.StatusOK
	}
	var verr config.ValidationError
	if errors.As(err, &verr) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}
