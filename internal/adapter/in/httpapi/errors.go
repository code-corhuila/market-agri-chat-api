package httpapi

import (
	"encoding/json"
	"net/http"
)

// Code is an error of the contract with its status and Spanish message (api-contract §2, D-C26).
type Code struct {
	Name    string
	Status  int
	Message string
}

// Codes shared by every service (api-contract §2.1).
var (
	CodeValidation         = Code{"VALIDATION_ERROR", http.StatusBadRequest, "La solicitud tiene datos inválidos."}
	CodeUnauthorized       = Code{"UNAUTHORIZED", http.StatusUnauthorized, "Debe iniciar sesión para continuar."}
	CodeForbidden          = Code{"FORBIDDEN", http.StatusForbidden, "No tiene permisos para realizar esta acción."}
	CodeNotFound           = Code{"NOT_FOUND", http.StatusNotFound, "El recurso solicitado no existe."}
	CodeMethodNotAllowed   = Code{"METHOD_NOT_ALLOWED", http.StatusMethodNotAllowed, "Método no permitido."}
	CodeServiceUnavailable = Code{"SERVICE_UNAVAILABLE", http.StatusServiceUnavailable, "El servicio no está disponible en este momento."}
	CodeInternal           = Code{"INTERNAL_ERROR", http.StatusInternalServerError, "Ocurrió un error inesperado."}
)

// FieldDetail names a field of the request and what is wrong with it.
type FieldDetail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// APIError is the one error body (api-contract §2, D-C21): details is always present, and traceId
// is the request's X-Correlation-Id.
type APIError struct {
	Error   string        `json:"error"`
	Message string        `json:"message"`
	Details []FieldDetail `json:"details"`
	TraceID string        `json:"traceId"`
}

// WriteError writes the envelope of code, with optional field details.
func WriteError(w http.ResponseWriter, r *http.Request, code Code, details ...FieldDetail) {
	if details == nil {
		details = []FieldDetail{}
	}
	WriteJSON(w, code.Status, APIError{
		Error: code.Name, Message: code.Message, Details: details, TraceID: CorrelationID(r.Context()),
	})
}

// WriteJSON writes v as the JSON body with status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
