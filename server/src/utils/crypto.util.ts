import * as crypto from 'crypto';

/**
 * 生成加密密钥
 * @returns 返回一个32字节长的AES-256密钥
 */
export function generateEncryptionKey(): Buffer {
  return crypto.randomBytes(32); // AES-256 requires 32-byte keys
}

/**
 * 加密数据
 * @param data 要加密的字符串数据
 * @param key 加密密钥
 * @returns 包含加密数据、初始化向量和认证标签的对象
 */
export function encrypt(data: string, key: Buffer): { 
  encryptedData: string; 
  iv: string; 
  authTag: string; 
  algorithm: string;
} {
  // 确保密钥长度正确
  if (key.length !== 32) {
    throw new Error('Encryption key must be 32 bytes for AES-256-GCM');
  }

  const iv = crypto.randomBytes(12); // GCM standard IV length is 96 bits = 12 bytes
  const cipher = crypto.createCipheriv('aes-256-gcm', key, iv);

  // 加密数据
  let encrypted = cipher.update(data, 'utf8', 'hex');
  encrypted += cipher.final('hex');

  // 获取认证标签
  const authTag = cipher.getAuthTag();

  return {
    encryptedData: encrypted,
    iv: iv.toString('hex'),
    authTag: authTag.toString('hex'),
    algorithm: 'aes-256-gcm'
  };
}

/**
 * 解密数据
 * @param encryptedObj 封装了加密数据的对象
 * @param key 解密密钥
 * @returns 解密后的原始字符串
 */
export function decrypt(encryptedObj: { 
  encryptedData: string; 
  iv: string; 
  authTag: string; 
}, key: Buffer): string {
  // 确保密钥长度正确
  if (key.length !== 32) {
    throw new Error('Decryption key must be 32 bytes for AES-256-GCM');
  }

  // 验证输入参数
  if (!encryptedObj.encryptedData || !encryptedObj.iv || !encryptedObj.authTag) {
    throw new Error('Invalid encrypted data format');
  }

  const ivBuffer = Buffer.from(encryptedObj.iv, 'hex');
  const decipher = crypto.createDecipheriv('aes-256-gcm', key, ivBuffer);
  decipher.setAuthTag(Buffer.from(encryptedObj.authTag, 'hex'));

  // 解密数据
  let decrypted = decipher.update(encryptedObj.encryptedData, 'hex', 'utf8');
  decrypted += decipher.final('utf8');

  return decrypted;
}

/**
 * 生成随机的初始化向量
 * @returns 返回一个随机的12字节IV
 */
export function generateRandomIV(): Buffer {
  return crypto.randomBytes(12);
}

/**
 * 生成哈希值
 * @param data 需要哈希的数据
 * @param algorithm 哈希算法，默认为sha256
 * @returns 返回十六进制形式的哈希值
 */
export function generateHash(data: string, algorithm: string = 'sha256'): string {
  return crypto.createHash(algorithm).update(data).digest('hex');
}

/**
 * 创建HMAC
 * @param data 要签名的数据
 * @param secret 密钥
 * @param algorithm 算法，默认为sha256
 * @returns 返回HMAC哈希值
 */
export function createHmac(data: string, secret: string, algorithm: string = 'sha256'): string {
  return crypto.createHmac(algorithm, secret).update(data).digest('hex');
}

/**
 * 使用PBKDF2派生密钥
 * @param password 用户密码
 * @param salt 盐值
 * @param iterations 迭代次数
 * @param keylen 密钥长度
 * @param digest 哈希算法
 * @returns 返回派生的密钥
 */
export function deriveKey(password: string, salt: Buffer, 
                         iterations: number = 100000, 
                         keylen: number = 32, 
                         digest: string = 'sha256'): Promise<Buffer> {
  return new Promise((resolve, reject) => {
    crypto.pbkdf2(password, salt, iterations, keylen, digest, (err, derivedKey) => {
      if (err) {
        reject(err);
      } else {
        resolve(derivedKey);
      }
    });
  });
}
