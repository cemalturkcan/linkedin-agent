package indexer

type RefusalError struct {
	Message string
}

func (e *RefusalError) Error() string {
	return e.Message
}

func refuse(message string) *RefusalError {
	return &RefusalError{Message: message}
}
