package model

// Error is a domain error with the code of the contract; the HTTP adapter maps each code to its
// status and Spanish message in one place.
type Error struct{ Code string }

func (e *Error) Error() string { return e.Code }

var (
	ErrInvalid              = &Error{"VALIDATION_ERROR"}
	ErrConversationNotFound = &Error{"CONVERSATION_NOT_FOUND"}
	ErrNotAParticipant      = &Error{"NOT_A_PARTICIPANT"}
	ErrProductNotFound      = &Error{"PRODUCT_NOT_FOUND"}
	ErrCatalogUnavailable   = &Error{"CATALOG_UNAVAILABLE"}
)
