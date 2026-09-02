// Binario auxiliar de healthcheck.
//
// A imagem final e distroless: nao tem shell, curl nem wget. Para que a
// instrucao HEALTHCHECK do Docker funcione, compilamos este binario
// minusculo no mesmo build e o copiamos junto do servidor.
//
// Alternativa comum seria usar uma imagem alpine so para ter o wget, mas
// isso significaria carregar um shell e um gerenciador de pacotes em
// producao apenas para um health check.
package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	client := &http.Client{Timeout: 3 * time.Second}

	resp, err := client.Get("http://127.0.0.1:" + port + "/health")
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck falhou: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck: status inesperado %d\n", resp.StatusCode)
		os.Exit(1)
	}

	os.Exit(0)
}
