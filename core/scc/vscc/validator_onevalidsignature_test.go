/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package vscc

import (
	"testing"

	"bytes"
	"fmt"
	"os"

	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
)

func createTx() (*common.Envelope, *peer.ProposalResponse, error) {
	ccid := &peer.ChaincodeID{Name: "foo", Version: "v1"}
	cis := &peer.ChaincodeInvocationSpec{ChaincodeSpec: &peer.ChaincodeSpec{ChaincodeId: ccid}}

	prop, _, err := utils.CreateProposalFromCIS(common.HeaderType_ENDORSER_TRANSACTION, util.GetTestChainID(), cis, sid)
	if err != nil {
		return nil, nil, err
	}

	presp, err := utils.CreateProposalResponse(prop.Header, prop.Payload, &peer.Response{Status: 200}, []byte("res"), nil, ccid, nil, id)
	if err != nil {
		return nil, nil, err
	}

	env, err := utils.CreateSignedTx(prop, id, presp)
	if err != nil {
		return nil, nil, err
	}

	return env, presp, err
}

func TestInit(t *testing.T) {
	v := new(ValidatorOneValidSignature)
	stub := shim.NewMockStub("validatoronevalidsignature", v)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		t.Fatalf("vscc init failed with %s", res.Message)
	}
}

func getSignedByMSPMemberPolicy(mspID string) ([]byte, error) {
	p := cauthdsl.SignedByMspMember(mspID)

	b, err := utils.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("Could not marshal policy, err %s", err)
	}

	return b, err
}

func TestInvoke(t *testing.T) {
	v := new(ValidatorOneValidSignature)
	stub := shim.NewMockStub("validatoronevalidsignature", v)

	// Failed path: Invalid arguments
	args := [][]byte{[]byte("dv")}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		t.Fatalf("vscc invoke should have failed")
		return
	}

	args = [][]byte{[]byte("dv"), []byte("tx")}
	args[1] = nil
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		t.Fatalf("vscc invoke should have failed")
		return
	}

	tx, presp, err := createTx()
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
		return
	}

	envBytes, err := utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
		return
	}

	expectVod := &sysccprovider.VsccOutputData{
		ProposalResponseData: [][]byte{presp.Payload},
	}
	expectVodBytes, err := utils.Marshal(expectVod)
	if err != nil {
		t.Fatalf("Marshal VsccOutputData failed, err %s", err)
		return
	}

	// good path: signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
		return
	}

	args = [][]byte{[]byte("dv"), envBytes, policy}
	res := stub.MockInvoke("1", args)
	if res.Status != shim.OK {
		t.Fatalf("vscc invoke returned err %s", err)
		return
	}

	if bytes.Compare(expectVodBytes, res.Payload) != 0 {
		t.Fatalf("vscc returned error payload")
		return
	}

	// bad path: signed by the wrong MSP
	policy, err = getSignedByMSPMemberPolicy("barf")
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
		return
	}

	args = [][]byte{[]byte("dv"), envBytes, policy}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		t.Fatalf("vscc invoke should have failed")
		return
	}
}

var id msp.SigningIdentity
var sid []byte
var mspid string
var chainId string = util.GetTestChainID()

func TestMain(m *testing.M) {
	var err error

	// setup the MSP manager so that we can sign/verify
	msptesttools.LoadMSPSetupForTesting()

	id, err = mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		fmt.Printf("GetSigningIdentity failed with err %s", err)
		os.Exit(-1)
	}

	sid, err = id.Serialize()
	if err != nil {
		fmt.Printf("Serialize failed with err %s", err)
		os.Exit(-1)
	}

	// determine the MSP identifier for the first MSP in the default chain
	var msp msp.MSP
	mspMgr := mspmgmt.GetManagerForChain(chainId)
	msps, err := mspMgr.GetMSPs()
	if err != nil {
		fmt.Printf("Could not retrieve the MSPs for the chain manager, err %s", err)
		os.Exit(-1)
	}
	if len(msps) == 0 {
		fmt.Printf("At least one MSP was expected")
		os.Exit(-1)
	}
	for _, m := range msps {
		msp = m
		break
	}
	mspid, err = msp.GetIdentifier()
	if err != nil {
		fmt.Printf("Failure getting the msp identifier, err %s", err)
		os.Exit(-1)
	}

	os.Exit(m.Run())
}
