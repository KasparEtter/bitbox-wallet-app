// Copyright 2018 Shift Devices AG
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bitbox

import (
	"fmt"

	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcutil/hdkeychain"
	"github.com/digitalbitbox/bitbox-wallet-app/backend/coins/btc"
	"github.com/digitalbitbox/bitbox-wallet-app/backend/coins/coin"
	"github.com/digitalbitbox/bitbox-wallet-app/backend/coins/eth"
	keystorePkg "github.com/digitalbitbox/bitbox-wallet-app/backend/keystore"
	"github.com/digitalbitbox/bitbox-wallet-app/backend/signing"
	"github.com/digitalbitbox/bitbox-wallet-app/util/errp"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/sirupsen/logrus"
)

type keystore struct {
	dbb           *Device
	configuration *signing.Configuration
	cosignerIndex int
	log           *logrus.Entry
}

// // Configuration implements keystore.Keystore.
// func (keystore *keystore) Configuration() *signing.Configuration {
// 	return keystore.configuration
// }

// CosignerIndex implements keystore.Keystore.
func (keystore *keystore) CosignerIndex() int {
	return keystore.cosignerIndex
}

// HasSecureOutput implements keystore.Keystore.
func (keystore *keystore) HasSecureOutput() bool {
	return keystore.dbb.channel != nil
}

// OutputAddress implements keystore.Keystore.
func (keystore *keystore) OutputAddress(
	keyPath signing.AbsoluteKeypath, scriptType signing.ScriptType, coin coin.Coin) error {
	if !keystore.HasSecureOutput() {
		panic("HasSecureOutput must be true")
	}
	return keystore.dbb.displayAddress(keyPath.Encode(), fmt.Sprintf("%s-%s", coin.Code(), string(scriptType)))
}

// ExtendedPublicKey implements keystore.Keystore.
func (keystore *keystore) ExtendedPublicKey(
	keyPath signing.AbsoluteKeypath) (*hdkeychain.ExtendedKey, error) {
	return keystore.dbb.xpub(keyPath.Encode())
}

func (keystore *keystore) signBTCTransaction(btcProposedTx *btc.ProposedTransaction) error {
	keystore.log.Info("Sign btc transaction")
	signatureHashes := [][]byte{}
	keyPaths := []string{}
	transaction := btcProposedTx.TXProposal.Transaction
	for index, txIn := range transaction.TxIn {
		spentOutput, ok := btcProposedTx.PreviousOutputs[txIn.PreviousOutPoint]
		if !ok {
			keystore.log.Panic("There needs to be exactly one output being spent per input!")
		}
		address := btcProposedTx.GetAddress(spentOutput.ScriptHashHex())
		isSegwit, subScript := address.ScriptForHashToSign()
		var signatureHash []byte
		if isSegwit {
			var err error
			signatureHash, err = txscript.CalcWitnessSigHash(subScript, btcProposedTx.SigHashes,
				txscript.SigHashAll, transaction, index, spentOutput.Value)
			if err != nil {
				return errp.Wrap(err, "Failed to calculate SegWit signature hash")
			}
			keystore.log.Debug("Calculated segwit signature hash")
		} else {
			var err error
			signatureHash, err = txscript.CalcSignatureHash(
				subScript, txscript.SigHashAll, transaction, index)
			if err != nil {
				return errp.Wrap(err, "Failed to calculate legacy signature hash")
			}
			keystore.log.Debug("Calculated legacy signature hash")
		}

		signatureHashes = append(signatureHashes, signatureHash)
		keyPaths = append(keyPaths, address.Configuration.AbsoluteKeypath().Encode())

		// Special serialization of the unsigned transaction for the mobile verification app.
		txIn.SignatureScript = subScript
	}

	signatures, err := keystore.dbb.Sign(btcProposedTx.TXProposal, signatureHashes, keyPaths)
	if isErrorAbort(err) {
		return errp.WithStack(keystorePkg.ErrSigningAborted)
	}
	if err != nil {
		return errp.WithMessage(err, "Failed to sign signature hash")
	}
	if len(signatures) != len(transaction.TxIn) {
		panic("number of signatures doesn't match number of inputs")
	}
	for i, signature := range signatures {
		signature := signature
		btcProposedTx.Signatures[i][keystore.CosignerIndex()] = &signature.Signature
	}
	return nil
}

func (keystore *keystore) signETHTransaction(txProposal *eth.TxProposal) error {
	signatureHashes := [][]byte{
		txProposal.Signer.Hash(txProposal.Tx).Bytes(),
	}
	_ = signatureHashes
	signatures, err := keystore.dbb.Sign(nil, signatureHashes, []string{txProposal.Keypath.Encode()})
	if isErrorAbort(err) {
		return errp.WithStack(keystorePkg.ErrSigningAborted)
	}
	if err != nil {
		return err
	}
	if len(signatures) != 1 {
		panic("expecting one signature")
	}
	signature := signatures[0]
	// We serialize the sig (including the recid at the last byte) so we can use WithSignature()
	// without modifications, even though it deserializes it again immediately. We do this because
	// it also modifies the `V` value according to EIP155.
	sig := make([]byte, 65)
	copy(sig[:32], math.PaddedBigBytes(signature.R, 32))
	copy(sig[32:64], math.PaddedBigBytes(signature.S, 32))
	sig[64] = byte(signature.RecID)
	signedTx, err := txProposal.Tx.WithSignature(txProposal.Signer, sig)
	if err != nil {
		return err
	}
	txProposal.Tx = signedTx
	return nil
}

// SignTransaction implements keystore.Keystore.
func (keystore *keystore) SignTransaction(proposedTx coin.ProposedTransaction) error {
	switch specificProposedTx := proposedTx.(type) {
	case *btc.ProposedTransaction:
		return keystore.signBTCTransaction(specificProposedTx)
	case *eth.TxProposal:
		return keystore.signETHTransaction(specificProposedTx)
	default:
		panic("unknown proposal type")
	}
}
