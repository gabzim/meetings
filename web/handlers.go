package web

import (
	"fmt"
	"net/http"
	"net/http/httputil"
)

const (
	greet = `<?xml version="1.0" encoding="UTF-8"?>
			<Response>
				<Say voice="woman">Hello thanks for picking up</Say>
				<Hangup />
			</Response>`
)

func PhonePickedUpHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	reqBytes, err := httputil.DumpRequest(r, true)
	if err != nil {
		fmt.Println(string(reqBytes))
	}

	w.Header().Set("Content-Type", "text/xml")

	fmt.Fprint(w, greet)
}
