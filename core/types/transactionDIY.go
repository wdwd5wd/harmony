package types

// SetPayload set a fixed-size payload to the tx
func (tx *Transaction) SetPayload() {
	size := make([]byte, 300)
	tx.data.Payload = size
}
