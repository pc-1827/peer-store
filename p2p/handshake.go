package p2p

type HandShakeFunc func(Peer) error

func NOTHandShakeFunc(Peer) error { return nil }
