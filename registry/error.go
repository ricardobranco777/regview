package registry

import (
	"bytes"
	"errors"

	"github.com/ricardobranco777/regview/oci"
)

// Check API error
func apiError(data []byte, err error) error {
	if err != nil {
		return err
	}
	if bytes.HasPrefix(data, []byte(`{"errors"`)) {
		var apiErr oci.ErrorResponse
		if err := apiErr.UnmarshalJSON(data); err != nil {
			return err
		}
		str := apiErr.Errors[0].Code
		if apiErr.Errors[0].Message != "" {
			str += ": " + apiErr.Errors[0].Message
		}
		return errors.New(str)
	}
	return nil
}
