package function

import (
	"context"
	"fmt"
	"net/http"
)

// Handle an HTTP Request.
func Handle(ctx context.Context, res http.ResponseWriter, req *http.Request) {
	fmt.Fprint(res, "Ash")
}
