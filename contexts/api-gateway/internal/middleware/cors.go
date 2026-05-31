package middleware

import "net/http"

var upstreamCORSHeaders = []string{
	"Access-Control-Allow-Origin",
	"Access-Control-Allow-Methods",
	"Access-Control-Allow-Headers",
	"Access-Control-Allow-Credentials",
	"Access-Control-Expose-Headers",
}

// CORSHandler centraliza CORS en el gateway.
// El corsWriter intercepta WriteHeader para limpiar cualquier header CORS
// que haya copiado el proxy del upstream y poner los propios, evitando duplicados.
func CORSHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			setCORSHeaders(w.Header())
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(&corsWriter{ResponseWriter: w}, r)
	})
}

func setCORSHeaders(h http.Header) {
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, X-User-Id, X-User-Role")
	h.Set("Access-Control-Expose-Headers", "Content-Length")
}

type corsWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

// WriteHeader se llama luego de que httputil.ReverseProxy copió los headers del upstream.
// Acá limpiamos los CORS del upstream y ponemos los nuestros antes de enviar al cliente.
func (cw *corsWriter) WriteHeader(status int) {
	if cw.wroteHeader {
		return
	}
	cw.wroteHeader = true
	h := cw.ResponseWriter.Header()
	for _, key := range upstreamCORSHeaders {
		h.Del(key)
	}
	setCORSHeaders(h)
	cw.ResponseWriter.WriteHeader(status)
}

func (cw *corsWriter) Write(b []byte) (int, error) {
	if !cw.wroteHeader {
		cw.WriteHeader(http.StatusOK)
	}
	return cw.ResponseWriter.Write(b)
}
