package api

import (
	"errors"
	"net"

	"github.com/OpenBazaar/multiwallet"
	"github.com/OpenBazaar/multiwallet/api/pb"
	"github.com/OpenBazaar/multiwallet/bitcoin"
	"github.com/OpenBazaar/multiwallet/bitcoincash"
	"github.com/OpenBazaar/wallet-interface"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

const Addr = "127.0.0.1:8234"

type server struct {
	w multiwallet.MultiWallet
}

func ServeAPI(w multiwallet.MultiWallet) error {
	lis, err := net.Listen("tcp", Addr)
	if err != nil {
		return err
	}
	s := grpc.NewServer()
	pb.RegisterAPIServer(s, &server{w})
	reflection.Register(s)
	if err := s.Serve(lis); err != nil {
		return err
	}
	return nil
}

func coinType(coinType pb.CoinType) wallet.CoinType {
	switch coinType {
	case pb.CoinType_BITCOIN:
		return wallet.Bitcoin
	case pb.CoinType_BITCOIN_CASH:
		return wallet.BitcoinCash
	case pb.CoinType_ZCASH:
		return wallet.Zcash
	case pb.CoinType_LITECOIN:
		return wallet.Litecoin
	default:
		return wallet.Bitcoin
	}
}

func (s *server) Stop(ctx context.Context, in *pb.Empty) (*pb.Empty, error) {
	// Stub
	return &pb.Empty{}, nil
}

func (s *server) CurrentAddress(ctx context.Context, in *pb.KeySelection) (*pb.Address, error) {
	var purpose wallet.KeyPurpose
	if in.Purpose == pb.KeyPurpose_INTERNAL {
		purpose = wallet.INTERNAL
	} else if in.Purpose == pb.KeyPurpose_EXTERNAL {
		purpose = wallet.EXTERNAL
	} else {
		return nil, errors.New("Unknown key purpose")
	}

	addr := s.w[coinType(in.Coin)].CurrentAddress(purpose)
	return &pb.Address{in.Coin, addr.String()}, nil
}

func (s *server) NewAddress(ctx context.Context, in *pb.KeySelection) (*pb.Address, error) {
	var purpose wallet.KeyPurpose
	if in.Purpose == pb.KeyPurpose_INTERNAL {
		purpose = wallet.INTERNAL
	} else if in.Purpose == pb.KeyPurpose_EXTERNAL {
		purpose = wallet.EXTERNAL
	} else {
		return nil, errors.New("Unknown key purpose")
	}
	addr := s.w[coinType(in.Coin)].NewAddress(purpose)
	return &pb.Address{in.Coin, addr.String()}, nil
}

func (s *server) ChainTip(ctx context.Context, in *pb.CoinSelection) (*pb.Height, error) {
	h, _ := s.w[coinType(in.Coin)].ChainTip()
	return &pb.Height{h}, nil
}

func (s *server) Balance(ctx context.Context, in *pb.CoinSelection) (*pb.Balances, error) {
	c, u := s.w[coinType(in.Coin)].Balance()
	return &pb.Balances{uint64(c), uint64(u)}, nil
}

func (s *server) MasterPrivateKey(ctx context.Context, in *pb.CoinSelection) (*pb.Key, error) {
	// Stub
	return &pb.Key{""}, nil
}

func (s *server) MasterPublicKey(ctx context.Context, in *pb.CoinSelection) (*pb.Key, error) {
	// Stub
	return &pb.Key{""}, nil
}

func (s *server) Params(ctx context.Context, in *pb.Empty) (*pb.NetParams, error) {
	// Stub
	return &pb.NetParams{""}, nil
}

func (s *server) HasKey(ctx context.Context, in *pb.Address) (*pb.BoolResponse, error) {
	// Stub
	return &pb.BoolResponse{false}, nil
}

func (s *server) Transactions(ctx context.Context, in *pb.CoinSelection) (*pb.TransactionList, error) {
	// Stub
	var list []*pb.Tx
	return &pb.TransactionList{list}, nil
}

func (s *server) GetTransaction(ctx context.Context, in *pb.Txid) (*pb.Tx, error) {
	// Stub
	respTx := &pb.Tx{}
	return respTx, nil
}

func (s *server) GetFeePerByte(ctx context.Context, in *pb.FeeLevelSelection) (*pb.FeePerByte, error) {
	// Stub
	return &pb.FeePerByte{0}, nil
}

func (s *server) Spend(ctx context.Context, in *pb.SpendInfo) (*pb.Txid, error) {
	var addr wallet.WalletAddress
	var err error
	switch in.Coin {
	case pb.CoinType_BITCOIN:
		addr, err = s.w[wallet.Bitcoin].DecodeAddress(in.Address)
	}
	if err != nil {
		return nil, err
	}
	var feeLevel wallet.FeeLevel
	switch in.FeeLevel {
	case pb.FeeLevel_PRIORITY:
		feeLevel = wallet.PRIOIRTY
	case pb.FeeLevel_NORMAL:
		feeLevel = wallet.NORMAL
	case pb.FeeLevel_ECONOMIC:
		feeLevel = wallet.ECONOMIC
	default:
		feeLevel = wallet.NORMAL
	}
	txid, err := s.w[coinType(in.Coin)].Spend(int64(in.Amount), addr, feeLevel)
	if err != nil {
		return nil, err
	}
	return &pb.Txid{in.Coin, txid.String()}, nil
}

func (s *server) BumpFee(ctx context.Context, in *pb.Txid) (*pb.Txid, error) {
	// Stub
	return &pb.Txid{in.Coin, ""}, nil
}

func (s *server) AddWatchedScript(ctx context.Context, in *pb.Address) (*pb.Empty, error) {
	return nil, nil
}

func (s *server) GetConfirmations(ctx context.Context, in *pb.Txid) (*pb.Confirmations, error) {
	// Stub
	return &pb.Confirmations{0}, nil
}

func (s *server) SweepAddress(ctx context.Context, in *pb.SweepInfo) (*pb.Txid, error) {
	// Stub
	return &pb.Txid{in.Coin, ""}, nil
}

func (s *server) CreateMultisigSignature(ctx context.Context, in *pb.CreateMultisigInfo) (*pb.SignatureList, error) {
	var retSigs []*pb.Signature
	return &pb.SignatureList{retSigs}, nil
}

func (s *server) Multisign(ctx context.Context, in *pb.MultisignInfo) (*pb.RawTx, error) {
	// Stub
	return &pb.RawTx{[]byte{}}, nil
}

func (s *server) EstimateFee(ctx context.Context, in *pb.EstimateFeeData) (*pb.Fee, error) {
	// Stub
	return &pb.Fee{0}, nil
}

func (s *server) WalletNotify(in *pb.CoinSelection, stream pb.API_WalletNotifyServer) error {
	// Stub
	return nil
}

func (s *server) GetKey(ctx context.Context, in *pb.Address) (*pb.Key, error) {
	// Stub
	return &pb.Key{""}, nil
}

func (s *server) ListAddresses(ctx context.Context, in *pb.CoinSelection) (*pb.Addresses, error) {
	// Stub
	var list []*pb.Address
	return &pb.Addresses{list}, nil
}

func (s *server) ListKeys(ctx context.Context, in *pb.CoinSelection) (*pb.Keys, error) {
	// Stub
	var list []*pb.Key
	return &pb.Keys{list}, nil
}

type HeaderWriter struct {
	stream pb.API_DumpTablesServer
}

func (h *HeaderWriter) Write(p []byte) (n int, err error) {
	hdr := &pb.Row{string(p)}
	if err := h.stream.Send(hdr); err != nil {
		return 0, err
	}
	return 0, nil
}

func (s *server) DumpTables(in *pb.CoinSelection, stream pb.API_DumpTablesServer) error {
	writer := HeaderWriter{stream}
	bitcoinWallet, ok := s.w[coinType(in.Coin)].(*bitcoin.BitcoinWallet)
	if ok {
		bitcoinWallet.DumpTables(&writer)
		return nil
	}
	bitcoincashWallet, ok := s.w[coinType(in.Coin)].(*bitcoincash.BitcoinCashWallet)
	if ok {
		bitcoincashWallet.DumpTables(&writer)
		return nil
	}
	return nil
}
