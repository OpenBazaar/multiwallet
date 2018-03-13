package cli

import (
	"errors"
	"fmt"
	"github.com/OpenBazaar/multiwallet/api"
	"github.com/OpenBazaar/multiwallet/api/pb"
	"github.com/jessevdk/go-flags"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"strings"
)

func SetupCli(parser *flags.Parser) {
	// Add commands to parser
	parser.AddCommand("stop",
		"stop the wallet",
		"The stop command disconnects from peers and shuts down the wallet",
		&stop)
	parser.AddCommand("currentaddress",
		"get the current bitcoin address",
		"Returns the first unused address in the keychain\n\n"+
			"Args:\n"+
			"1. coinType (string)\n"+
			"2. purpose       (string default=external) The purpose for the address. Can be external for receiving from outside parties or internal for example, for change.\n\n"+
			"Examples:\n"+
			"> multiwallet currentaddress bitcoin\n"+
			"1DxGWC22a46VPEjq8YKoeVXSLzB7BA8sJS\n"+
			"> multiwallet currentaddress bitcoin internal\n"+
			"18zAxgfKx4NuTUGUEuB8p7FKgCYPM15DfS\n",
		&currentAddress)
	parser.AddCommand("newaddress",
		"get a new bitcoin address",
		"Returns a new unused address in the keychain. Use caution when using this function as generating too many new addresses may cause the keychain to extend further than the wallet's lookahead window, meaning it might fail to recover all transactions when restoring from seed. CurrentAddress is safer as it never extends past the lookahead window.\n\n"+
			"Args:\n"+
			"1. coinType (string)\n"+
			"2. purpose       (string default=external) The purpose for the address. Can be external for receiving from outside parties or internal for example, for change.\n\n"+
			"Examples:\n"+
			"> multiwallet newaddress bitcoin\n"+
			"1DxGWC22a46VPEjq8YKoeVXSLzB7BA8sJS\n"+
			"> multiwallet newaddress bitcoin internal\n"+
			"18zAxgfKx4NuTUGUEuB8p7FKgCYPM15DfS\n",
		&newAddress)
	parser.AddCommand("chaintip",
		"return the height of the chain",
		"Returns the height of the best chain of blocks",
		&chainTip)
	parser.AddCommand("dumptables",
		"print out the database tables",
		"Prints each row in the database tables",
		&dumpTables)
}

func coinType(args []string) pb.CoinType {
	if len(args) == 0 {
		return pb.CoinType_BITCOIN
	}
	switch strings.ToLower(args[0]) {
	case "bitcoin":
		return pb.CoinType_BITCOIN
	case "bitcoincash":
		return pb.CoinType_BITCOIN_CASH
	case "zcash":
		return pb.CoinType_ZCASH
	case "litecoin":
		return pb.CoinType_LITECOIN
	default:
		return pb.CoinType_BITCOIN
	}
}

func newGRPCClient() (pb.APIClient, *grpc.ClientConn, error) {
	// Set up a connection to the server.
	conn, err := grpc.Dial(api.Addr, grpc.WithInsecure())
	if err != nil {
		return nil, nil, err
	}
	client := pb.NewAPIClient(conn)
	return client, conn, nil
}

type Stop struct{}

var stop Stop

func (x *Stop) Execute(args []string) error {
	client, conn, err := newGRPCClient()
	if err != nil {
		return err
	}
	defer conn.Close()
	client.Stop(context.Background(), &pb.Empty{})
	return nil
}

type CurrentAddress struct{}

var currentAddress CurrentAddress

func (x *CurrentAddress) Execute(args []string) error {
	client, conn, err := newGRPCClient()
	if err != nil {
		return err
	}
	defer conn.Close()
	var purpose pb.KeyPurpose
	userSelection := ""

	t := coinType(args)
	if len(args) == 1 {
		userSelection = args[0]
	} else if len(args) == 2 {
		userSelection = args[1]
	}
	switch strings.ToLower(userSelection) {
	case "internal":
		purpose = pb.KeyPurpose_INTERNAL
	case "external":
		purpose = pb.KeyPurpose_EXTERNAL
	default:
		purpose = pb.KeyPurpose_EXTERNAL
	}

	resp, err := client.CurrentAddress(context.Background(), &pb.KeySelection{t, purpose})
	if err != nil {
		return err
	}
	fmt.Println(resp.Addr)
	return nil
}

type NewAddress struct{}

var newAddress NewAddress

func (x *NewAddress) Execute(args []string) error {
	client, conn, err := newGRPCClient()
	if err != nil {
		return err
	}
	defer conn.Close()
	if len(args) == 0 {
		return errors.New("Must select coin type")
	}
	t := coinType(args)
	var purpose pb.KeyPurpose
	userSelection := ""
	if len(args) == 1 {
		userSelection = args[0]
	} else if len(args) == 2 {
		userSelection = args[1]
	}
	switch strings.ToLower(userSelection) {
	case "internal":
		purpose = pb.KeyPurpose_INTERNAL
	case "external":
		purpose = pb.KeyPurpose_EXTERNAL
	default:
		purpose = pb.KeyPurpose_EXTERNAL
	}
	resp, err := client.NewAddress(context.Background(), &pb.KeySelection{t, purpose})
	if err != nil {
		return err
	}
	fmt.Println(resp.Addr)
	return nil
}

type ChainTip struct{}

var chainTip ChainTip

func (x *ChainTip) Execute(args []string) error {
	client, conn, err := newGRPCClient()
	if err != nil {
		return err
	}
	defer conn.Close()
	if len(args) == 0 {
		return errors.New("Must select coin type")
	}
	t := coinType(args)
	resp, err := client.ChainTip(context.Background(), &pb.CoinSelection{t})
	if err != nil {
		return err
	}
	fmt.Println(resp.Height)
	return nil
}

type DumpTables struct{}

var dumpTables DumpTables

func (x *DumpTables) Execute(args []string) error {
	client, conn, err := newGRPCClient()
	if err != nil {
		return err
	}
	defer conn.Close()
	if len(args) == 0 {
		return errors.New("Must select coin type")
	}
	t := coinType(args)
	resp, err := client.DumpTables(context.Background(), &pb.CoinSelection{t})
	if err != nil {
		return err
	}
	for {
		row, err := resp.Recv()
		if err != nil {
			return err
		}
		fmt.Println(row.Data)
	}
	return nil
}
