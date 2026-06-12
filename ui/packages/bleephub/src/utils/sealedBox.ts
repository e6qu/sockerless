import sodium from "libsodium-wrappers";

/**
 * Seal a secret value for the GitHub Actions secrets API: libsodium
 * sealed box (crypto_box_seal) against the scope's X25519 public key,
 * base64 in and out — the exact scheme real GitHub documents for
 * PUT .../secrets/{name}. The plaintext never goes on the wire.
 */
export async function sealSecret(plaintext: string, publicKeyB64: string): Promise<string> {
  await sodium.ready;
  // Pass the string straight through: libsodium UTF-8-encodes string
  // messages itself. A TextEncoder-produced Uint8Array can fail the
  // library's instanceof check when realms differ (e.g. jsdom).
  const sealed = sodium.crypto_box_seal(
    plaintext,
    sodium.from_base64(publicKeyB64, sodium.base64_variants.ORIGINAL),
  );
  return sodium.to_base64(sealed, sodium.base64_variants.ORIGINAL);
}
