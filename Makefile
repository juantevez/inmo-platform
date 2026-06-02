.PHONY: test test-cover test-html

# Corre los tests normales de forma rápida
test:
	go test ./... -v

# Muestra el porcentaje de coverage directo en la consola
test-cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out

# Genera el reporte y te abre el navegador automáticamente para ver las líneas en rojo/verde
test-html: test-cover
	go tool cover -html=coverage.out