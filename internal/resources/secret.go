/*
Copyright 2026.

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

package resources

import (
	"crypto/rand"
	"math/big"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	databasev1alpha1 "github.com/4thel00z/serenedb-operator/api/v1alpha1"
)

const passwordAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
const passwordLength = 24

// GeneratedSecret builds the superuser password Secret with a fresh random password.
func GeneratedSecret(db *databasev1alpha1.SereneDB) (*corev1.Secret, error) {
	password, err := randomPassword()
	if err != nil {
		return nil, err
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      Name(db),
			Namespace: db.Namespace,
			Labels:    Labels(db),
		},
		Type:       corev1.SecretTypeOpaque,
		StringData: map[string]string{PasswordKey(db): password},
	}, nil
}

// SecretOwnedByDatabase reports whether the generated Secret should be garbage collected with the SereneDB.
// The password is only honored on first boot, so it must outlive the SereneDB whenever the data volume does.
func SecretOwnedByDatabase(db *databasev1alpha1.SereneDB) bool {
	return db.Spec.Persistence.RetentionPolicy.WhenDeleted == "Delete"
}

func randomPassword() (string, error) {
	out := make([]byte, passwordLength)
	max := big.NewInt(int64(len(passwordAlphabet)))
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = passwordAlphabet[n.Int64()]
	}
	return string(out), nil
}
