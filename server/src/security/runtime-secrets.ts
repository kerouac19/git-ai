import * as crypto from 'crypto';

let generatedEncryptionMasterKey: Buffer | null = null;
let generatedCasEncryptionKey: string | null = null;

function isProductionEnvironment() {
  return process.env.NODE_ENV === 'production';
}

export function resolveEncryptionMasterKey() {
  const configuredKey = process.env.ENCRYPTION_MASTER_KEY?.trim();
  if (configuredKey) {
    const keyBuffer = Buffer.from(configuredKey, 'hex');
    if (keyBuffer.length !== 32) {
      throw new Error('ENCRYPTION_MASTER_KEY must be a 64-character hex string');
    }

    return keyBuffer;
  }

  if (isProductionEnvironment()) {
    throw new Error('ENCRYPTION_MASTER_KEY must be set in production');
  }

  if (!generatedEncryptionMasterKey) {
    generatedEncryptionMasterKey = crypto.randomBytes(32);
  }

  return generatedEncryptionMasterKey;
}

export function resolveCasEncryptionKey() {
  const configuredKey = process.env.CAS_ENCRYPTION_KEY?.trim();
  if (configuredKey) {
    return configuredKey;
  }

  if (isProductionEnvironment()) {
    throw new Error('CAS_ENCRYPTION_KEY must be set in production');
  }

  if (!generatedCasEncryptionKey) {
    generatedCasEncryptionKey = crypto.randomBytes(32).toString('hex');
  }

  return generatedCasEncryptionKey;
}
