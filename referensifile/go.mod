// Separate module — referensifile/ adalah READ-ONLY reference code.
// Tidak di-compile dengan parent module (flowork-gui).
// Tujuan: cegah `go build ./...` dan `go mod tidy` parent scan ke sini.
module referensifile-readonly

go 1.25.0
