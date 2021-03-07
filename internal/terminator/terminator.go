package terminator

var StopSig chan struct{} = make(chan struct{}, STOPSIG_SIZE)

const STOPSIG_SIZE = 65536

func Terminator() {

	for i := 0; i != STOPSIG_SIZE; i++ {
		StopSig <- struct{}{}
	}
	return
}
