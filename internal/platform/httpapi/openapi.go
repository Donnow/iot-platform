package httpapi

import (
	_ "embed"
	"errors"
	"net/http"
)

//go:embed openapi.yaml
var openAPISpec []byte

func serveOpenAPI(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	writer.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(openAPISpec)
}

func serveSwaggerUI(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	const page = `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>IoT Platform API</title><link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css"></head>
<body><div id="swagger-ui"></div><script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>window.ui=SwaggerUIBundle({url:'/openapi.yaml',dom_id:'#swagger-ui',deepLinking:true});</script></body></html>`
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte(page))
}
