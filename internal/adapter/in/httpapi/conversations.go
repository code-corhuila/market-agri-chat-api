package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/code-corhuila/market-agri-chat-api/internal/application/port/in"
	"github.com/code-corhuila/market-agri-chat-api/internal/domain/model"
)

// uuidPattern: identifiers are lower-case UUIDs (api-contract §1).
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// domainErrors maps each domain code to its status and Spanish message: one place (cards E-30,
// E-32). A code without a row is 500 and the code stays in the log.
var domainErrors = map[string]Code{
	"VALIDATION_ERROR":       CodeValidation,
	"CONVERSATION_NOT_FOUND": {"CONVERSATION_NOT_FOUND", http.StatusNotFound, "La conversación no existe."},
	"NOT_A_PARTICIPANT":      {"NOT_A_PARTICIPANT", http.StatusForbidden, "No participa en esta conversación."},
	"PRODUCT_NOT_FOUND":      {"PRODUCT_NOT_FOUND", http.StatusNotFound, "El producto no existe."},
	"CATALOG_UNAVAILABLE":    {"CATALOG_UNAVAILABLE", http.StatusServiceUnavailable, "El catálogo no está disponible. Intente de nuevo."},
}

// ConversationView is the one shape of a conversation in E-30…E-34 and I-05 (api-contract §4.3).
type ConversationView struct {
	ID                   string  `json:"id"`
	ProductID            string  `json:"productId"`
	ProductName          string  `json:"productName"`
	BuyerID              string  `json:"buyerId"`
	BuyerName            string  `json:"buyerName"`
	ProducerID           string  `json:"producerId"`
	ProducerName         string  `json:"producerName"`
	AgreedPurchaseMethod *string `json:"agreedPurchaseMethod"`
	LastMessageAt        *string `json:"lastMessageAt"`
	CreatedAt            string  `json:"createdAt"`
}

func viewOf(c model.Conversation) ConversationView {
	v := ConversationView{
		ID: c.ID, ProductID: c.ProductID, ProductName: c.ProductName, BuyerID: c.BuyerID, BuyerName: c.BuyerName,
		ProducerID: c.ProducerID, ProducerName: c.ProducerName, CreatedAt: timestamp(c.CreatedAt),
	}
	if c.AgreedPurchaseMethod != nil {
		m := string(*c.AgreedPurchaseMethod)
		v.AgreedPurchaseMethod = &m
	}
	if c.LastMessageAt != nil {
		t := timestamp(*c.LastMessageAt)
		v.LastMessageAt = &t
	}
	return v
}

// timestamp: ISO 8601 in UTC with an explicit offset (api-contract §1).
func timestamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// ConversationRoutes mounts E-30, E-31, E-32 and I-05.
func ConversationRoutes(auth Auth, conversations in.Conversations, maxBody int64) func(*http.ServeMux) {
	return func(mux *http.ServeMux) {
		mux.HandleFunc("POST /api/v1/chat/conversations", auth.User([]string{"BUYER"}, func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				ProductID string `json:"productId"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&body); err != nil || !uuidPattern.MatchString(body.ProductID) {
				WriteError(w, r, CodeValidation, FieldDetail{"productId", "Debe ser un identificador válido."})
				return
			}
			caller := CallerFrom(r.Context())
			c, created, err := conversations.Open(r.Context(), in.Buyer{ID: caller.UserID, Name: caller.Name}, body.ProductID)
			if err != nil {
				writeDomainError(w, r, err)
				return
			}
			status := http.StatusOK
			if created {
				status = http.StatusCreated
				w.Header().Set("Location", "/api/v1/chat/conversations/"+c.ID)
			}
			WriteJSON(w, status, viewOf(c))
		}))

		mux.HandleFunc("GET /api/v1/chat/conversations", auth.User([]string{"PRODUCER", "BUYER"}, func(w http.ResponseWriter, r *http.Request) {
			page, limit, details := pagination(r.URL.Query())
			if details != nil {
				WriteError(w, r, CodeValidation, details...)
				return
			}
			result, err := conversations.List(r.Context(), CallerFrom(r.Context()).UserID, page, limit)
			if err != nil {
				writeDomainError(w, r, err)
				return
			}
			data := make([]ConversationView, 0, len(result.Items))
			for _, c := range result.Items {
				data = append(data, viewOf(c))
			}
			WriteJSON(w, http.StatusOK, map[string]any{"data": data, "meta": meta(page, limit, result.Total)})
		}))

		mux.HandleFunc("GET /api/v1/chat/conversations/{id}", auth.User([]string{"PRODUCER", "BUYER"}, func(w http.ResponseWriter, r *http.Request) {
			// A malformed {id} finds nothing, so it is 404 like a missing one (the card of E-32).
			c, err := conversations.Get(r.Context(), CallerFrom(r.Context()).UserID, r.PathValue("id"))
			if err != nil {
				writeDomainError(w, r, err)
				return
			}
			WriteJSON(w, http.StatusOK, viewOf(c))
		}))

		mux.HandleFunc("GET /internal/v1/chat/conversations/{id}", auth.Service(func(w http.ResponseWriter, r *http.Request) {
			c, err := conversations.GetForService(r.Context(), r.PathValue("id"))
			if err != nil {
				writeDomainError(w, r, err)
				return
			}
			WriteJSON(w, http.StatusOK, viewOf(c))
		}))
	}
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var domain *model.Error
	if errors.As(err, &domain) {
		if code, ok := domainErrors[domain.Code]; ok {
			WriteError(w, r, code)
			return
		}
	}
	loggerFrom(r.Context()).Error("Unhandled error", "error", err.Error())
	WriteError(w, r, CodeInternal)
}

const (
	defaultLimit = 20
	maxLimit     = 100
)

// pagination reads page and limit (D-C24): out of range, not a number or an unknown parameter is 400.
func pagination(q url.Values) (page, limit int, details []FieldDetail) {
	page, limit = 1, defaultLimit
	for name := range q {
		if name != "page" && name != "limit" {
			details = append(details, FieldDetail{name, "Parámetro desconocido."})
		}
	}
	if v := q.Get("page"); q.Has("page") {
		if n, err := strconv.Atoi(v); err != nil || n < 1 {
			details = append(details, FieldDetail{"page", "Debe ser un entero mayor o igual a 1."})
		} else {
			page = n
		}
	}
	if v := q.Get("limit"); q.Has("limit") {
		if n, err := strconv.Atoi(v); err != nil || n < 1 || n > maxLimit {
			details = append(details, FieldDetail{"limit", "Debe ser un entero entre 1 y 100."})
		} else {
			limit = n
		}
	}
	return page, limit, details
}

func meta(page, limit, total int) map[string]int {
	return map[string]int{"page": page, "limit": limit, "total": total, "totalPages": (total + limit - 1) / limit}
}
