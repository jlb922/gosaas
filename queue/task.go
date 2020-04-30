package queue

import (
	"errors"
	"fmt"
	"reflect"
	"time"
)

// TaskID are IDs representing a specific queued task. Each application task has an associated ID
type TaskID int

const (
	// TaskEmail is for sending email.
	TaskEmail TaskID = iota
	// TaskCreateInvoice is for creating new Stripe invoice.
	TaskCreateInvoice
)

// QueueTask represents a queued task.
//
// The Data field contains the necessary data for the task to execute properly.
type QueueTask struct {
	ID      TaskID      `json:"id"`
	Data    interface{} `json:"data"`
	Created time.Time   `json:"created"`
}

// TaskExecutor is an interface used to execute tasks based on their ID.
type TaskExecutor interface {
	Run(t QueueTask) error
}

// Since we’re wrapping a struct inside a QueueTask as an interface, they’re serialized
// as a map[string]interface{}. We need a way to convert that map back
// to the original struct. setField and fillStructs do that
func setField(obj interface{}, name string, value interface{}) error {
	structValue := reflect.ValueOf(obj).Elem()
	structFieldValue := structValue.FieldByName(name)

	if !structFieldValue.IsValid() {
		return fmt.Errorf("no such field: %s in obj", name)
	}

	if !structFieldValue.CanSet() {
		return fmt.Errorf("cannot set %s field value", name)
	}

	structFieldType := structFieldValue.Type()
	val := reflect.ValueOf(value)
	if structFieldType != val.Type() {
		invalidTypeError := errors.New("provided value type didn't match obj field type")
		return invalidTypeError
	}

	structFieldValue.Set(val)
	return nil
}

// fillStruct receives a pointer to a structure to be filled from the map.
// Each entry in the map calls setField() to set the value of that field in
// the structure
func fillStruct(s interface{}, m map[string]interface{}) error {
	for k, v := range m {
		err := setField(s, k, v)
		if err != nil {
			return err
		}
	}
	return nil
}
