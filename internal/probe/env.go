package probe

// Env agrupa las dependencias sustituibles en tests. Dialer se declara en
// ports.go.
type Env struct {
	Echoer   Echoer
	Router   Router
	Resolver Resolver
	Clock    Clock
	Dialer   Dialer
}
