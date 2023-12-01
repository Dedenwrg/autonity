package common

var (
	EcrecoverAddress               = BytesToAddress([]byte{1})
	Sha256hashAddress              = BytesToAddress([]byte{2})
	Ripemd160hashAddress           = BytesToAddress([]byte{3})
	DataCopyAddress                = BytesToAddress([]byte{4})
	BigModExpAddress               = BytesToAddress([]byte{5})
	Bn256AddByzantiumAddress       = BytesToAddress([]byte{6})
	Bn256AddIstanbulAddress        = BytesToAddress([]byte{6})
	Bn256ScalarMulByzantiumAddress = BytesToAddress([]byte{7})
	Bn256ScalarMulIstanbulAddress  = BytesToAddress([]byte{7})
	Bn256PairingByzantiumAddress   = BytesToAddress([]byte{8})
	Bn256PairingIstanbulAddress    = BytesToAddress([]byte{8})
	Blake2FAddress                 = BytesToAddress([]byte{9})
	Bls12381G1AddAddress           = BytesToAddress([]byte{10})
	Bls12381G1MulAddress           = BytesToAddress([]byte{11})
	Bls12381G1MultiExpAddress      = BytesToAddress([]byte{12})
	Bls12381G2AddAddress           = BytesToAddress([]byte{13})
	Bls12381G2MulAddress           = BytesToAddress([]byte{14})
	Bls12381G2MultiExpAddress      = BytesToAddress([]byte{15})
	Bls12381PairingAddress         = BytesToAddress([]byte{16})
	Bls12381MapG1Address           = BytesToAddress([]byte{17})
	Bls12381MapG2Address           = BytesToAddress([]byte{18})
	CheckAccusationAddress         = BytesToAddress([]byte{252})
	CheckInnocenceAddress          = BytesToAddress([]byte{253})
	CheckMisbehaviourAddress       = BytesToAddress([]byte{254})
	CheckEnodeAddress              = BytesToAddress([]byte{255})
)
