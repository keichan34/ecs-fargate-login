/*
Copyright © 2019 Keitaroh Kobayashi

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/

package utils

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"log"
	"os"

	"golang.org/x/crypto/ssh"
)

// SSHKeyPair stores the SSH key pair in string form.
type SSHKeyPair struct {
	PrivateKeyPEM          string
	PublicKeyAuthorizedKey string
}

// generateSSHPrivateKey will generate an 2048-bit RSA key pair.
func generateSSHPrivateKey() (*rsa.PrivateKey, error) {
	// Private Key generation
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	log.Println("Private Key generated")
	return privateKey, nil
}

// GenerateSSHKeyPair returns a SSHKeyPair with a randomly generated RSA key pair.
func GenerateSSHKeyPair() (*SSHKeyPair, error) {
	privateKey, err := generateSSHPrivateKey()
	if err != nil {
		return nil, err
	}

	privateKeyBytes, err := ssHPrivateKeyToPEM(privateKey)
	if err != nil {
		return nil, err
	}

	publicKeyBytes, err := ssHPrivateKeyToAuthorizedPublicKey(privateKey)
	if err != nil {
		return nil, err
	}

	out := &SSHKeyPair{
		PrivateKeyPEM:          string(privateKeyBytes),
		PublicKeyAuthorizedKey: string(publicKeyBytes),
	}
	return out, nil
}

// WritePrivateKeyToTempfile will write the private key part of a key pair to a temporary file.
// You are responsible for removing the private key when you're done with it.
func WritePrivateKeyToTempfile(pair *SSHKeyPair) (*os.File, error) {
	tmpfile, err := ioutil.TempFile("", "tmpsshkey")
	if err != nil {
		return nil, err
	}
	if _, err := tmpfile.Write([]byte(pair.PrivateKeyPEM)); err != nil {
		os.Remove(tmpfile.Name())
		return nil, err
	}
	return tmpfile, nil
}

// ssHPrivateKeyToPEM encodes the private key in PEM format.
func ssHPrivateKeyToPEM(privateKey *rsa.PrivateKey) ([]byte, error) {
	// Get ASN.1 DER format
	privDER := x509.MarshalPKCS1PrivateKey(privateKey)

	// pem.Block
	privBlock := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   privDER,
	}

	// Private key in PEM format
	privatePEM := pem.EncodeToMemory(&privBlock)

	return privatePEM, nil
}

// ssHPrivateKeyToAuthorizedPublicKey encodes a
func ssHPrivateKeyToAuthorizedPublicKey(privatekey *rsa.PrivateKey) ([]byte, error) {
	publicKey, err := ssh.NewPublicKey(privatekey.Public())
	if err != nil {
		return nil, err
	}

	pubKeyBytes := ssh.MarshalAuthorizedKey(publicKey)

	log.Println("Public key generated")
	return pubKeyBytes, nil
}
