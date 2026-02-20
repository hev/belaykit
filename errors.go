package rack

import "errors"

// ErrNoJSON indicates no JSON object or array was found in the response.
var ErrNoJSON = errors.New("no JSON found in response")
