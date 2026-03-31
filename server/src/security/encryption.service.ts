import { Injectable, Logger, HttpException, HttpStatus } from '@nestjs/common';
import { generateEncryptionKey, encrypt, decrypt, generateHash, createHmac, deriveKey } from '../utils/crypto.util';
import { SECURITY_CONFIG } from '../config/security.config';
import { resolveEncryptionMasterKey } from './runtime-secrets';

@Injectable()
export class EncryptionService {
  private readonly logger = new Logger(EncryptionService.name);
  private readonly masterKey: Buffer;

  constructor() {
    // 初始化主密钥，实际生产中应该从安全的密钥管理服务获取
    try {
      this.masterKey = resolveEncryptionMasterKey();
      
      // 确保主密钥长度正确
      if (this.masterKey.length !== 32) {
        throw new Error('Invalid master key length for encryption');
      }
      
      if (!process.env.ENCRYPTION_MASTER_KEY?.trim()) {
        this.logger.warn('ENCRYPTION_MASTER_KEY is not set; using an ephemeral development key');
      }

      this.logger.log('Encryption service initialized successfully');
    } catch (error) {
      this.logger.error(`Failed to initialize encryption service: ${error.message}`);
      throw error;
    }
  }

  /**
   * 加密数据
   * @param data 要加密的数据
   * @param context 上下文信息，可能用于更复杂的加密场景
   * @returns 加密结果
   */
  async encryptData(data: any, context?: string): Promise<any> {
    try {
      // 根据数据类型进行适当处理
      const dataString = typeof data === 'string' ? data : JSON.stringify(data);
      
      // 执行加密
      const encryptedResult = encrypt(dataString, this.masterKey);
      
      // 记录到安全日志，但不包含实际数据
      this.logger.log(`Data encrypted${context ? ` with context: ${context}` : ''}`);
      
      return encryptedResult;
    } catch (error) {
      this.logger.error(`Encryption failed: ${error.message}`);
      throw new HttpException(
        'Encryption failed due to internal error',
        HttpStatus.INTERNAL_SERVER_ERROR
      );
    }
  }

  /**
   * 解密数据
   * @param encryptedData 加密的数据
   * @param context 上下文信息
   * @returns 解密后的数据
   */
  async decryptData(encryptedData: any, context?: string): Promise<any> {
    try {
      // 验证加密数据格式
      if (!encryptedData || !encryptedData.encryptedData || !encryptedData.iv || !encryptedData.authTag) {
        throw new Error('Invalid encrypted data format');
      }

      // 执行解密
      const decryptedString = decrypt(encryptedData, this.masterKey);
      
      // 尝试解析JSON，如果不是JSON则返回原始字符串
      try {
        return JSON.parse(decryptedString);
      } catch (parseError) {
        // 如果解析失败，则返回原始字符串
        return decryptedString;
      }
    } catch (error) {
      this.logger.error(`Decryption failed: ${error.message}`);
      throw new HttpException(
        'Decryption failed due to invalid data or internal error',
        HttpStatus.INTERNAL_SERVER_ERROR
      );
    }
  }

  /**
   * 加密特定字段
   * @param fieldName 字段名
   * @param value 字段值
   * @returns 加密后的数据
   */
  async encryptField(fieldName: string, value: any): Promise<any> {
    try {
      // 验证是否需要对这个字段进行加密
      const shouldEncrypt = SECURITY_CONFIG.ENCRYPTION.ENCRYPT_SENSITIVE_FIELDS.some(
        field => fieldName.toLowerCase().includes(field)
      );

      if (!shouldEncrypt) {
        this.logger.warn(`Field ${fieldName} might not be sensitive. Encrypt anyway?`);
      }

      // 加密字段值
      return await this.encryptData(value, `field:${fieldName}`);
    } catch (error) {
      this.logger.error(`Field encryption failed for ${fieldName}: ${error.message}`);
      throw error;
    }
  }

  /**
   * 批量加密功能
   * @param data 包含需要加密的数据的对象
   * @param fieldsToEncrypt 需要加密的字段列表
   */
  async bulkEncrypt(data: Record<string, any>, fieldsToEncrypt: string[]): Promise<Record<string, any>> {
    try {
      const result = { ...data };
      
      for (const field of fieldsToEncrypt) {
        if (field in result) {
          result[field] = await this.encryptData(result[field], `bulk-encrypt:${field}`);
        }
      }
      
      this.logger.log(`${fieldsToEncrypt.length} fields encrypted in bulk operation`);
      return result;
    } catch (error) {
      this.logger.error(`Bulk encryption failed: ${error.message}`);
      throw new HttpException(
        'Bulk encryption failed',
        HttpStatus.INTERNAL_SERVER_ERROR
      );
    }
  }

  /**
   * 批量解密功能
   * @param data 包含已加密数据的对象
   * @param fieldsToDecrypt 需要解密的字段列表
   */
  async bulkDecrypt(data: Record<string, any>, fieldsToDecrypt: string[]): Promise<Record<string, any>> {
    try {
      const result = { ...data };
      
      for (const field of fieldsToDecrypt) {
        if (field in result) {
          try {
            result[field] = await this.decryptData(result[field], `bulk-decrypt:${field}`);
          } catch (decryptError) {
            this.logger.warn(`Could not decrypt field ${field} in bulk operation: ${decryptError.message}`);
            // 保持原始加密数据不变
          }
        }
      }
      
      this.logger.log(`${fieldsToDecrypt.length} fields processed in bulk decryption operation`);
      return result;
    } catch (error) {
      this.logger.error(`Bulk decryption failed: ${error.message}`);
      throw new HttpException(
        'Bulk decryption failed',
        HttpStatus.INTERNAL_SERVER_ERROR
      );
    }
  }

  /**
   * 生成数据哈希
   * @param data 要哈希的数据
   * @param algorithm 哈希算法
   * @returns 哈希字符串
   */
  async generateHash(data: string, algorithm?: string): Promise<string> {
    try {
      return generateHash(data, algorithm);
    } catch (error) {
      this.logger.error(`Hash generation failed: ${error.message}`);
      throw new HttpException(
        'Hash generation failed',
        HttpStatus.INTERNAL_SERVER_ERROR
      );
    }
  }

  /**
   * 生成HMAC签名
   * @param data 要签名的数据
   * @param secret 密钥
   * @param algorithm 算法类型
   * @returns HMAC字符串
   */
  async generateHmac(data: string, secret?: string, algorithm?: string): Promise<string> {
    try {
      const actualSecret = secret || process.env.HMAC_SECRET || this.masterKey.toString('hex');
      return createHmac(data, actualSecret, algorithm);
    } catch (error) {
      this.logger.error(`HMAC generation failed: ${error.message}`);
      throw new HttpException(
        'HMAC generation failed',
        HttpStatus.INTERNAL_SERVER_ERROR
      );
    }
  }

  /**
   * 从密码派生密钥
   * @param password 用户密码
   * @param salt 盐值
   * @param iterations 迭代次数
   */
  async deriveKeyFromPassword(password: string, salt: Buffer, iterations?: number): Promise<Buffer> {
    try {
      const actualIterations = iterations || SECURITY_CONFIG.ENCRYPTION.KEY_ROTATION_INTERVAL_DAYS * 1000;
      return await deriveKey(password, salt, actualIterations);
    } catch (error) {
      this.logger.error(`Key derivation failed: ${error.message}`);
      throw new HttpException(
        'Key derivation failed',
        HttpStatus.INTERNAL_SERVER_ERROR
      );
    }
  }

  /**
   * 检查敏感数据分类
   * @param key 数据键名
   * @returns 是否属于敏感数据
   */
  isSensitiveData(key: string): boolean {
    if (!key) return false;
    return SECURITY_CONFIG.ENCRYPTION.ENCRYPT_SENSITIVE_FIELDS.some(
      field => key.toLowerCase().includes(field.toLowerCase())
    );
  }

  /**
   * 获取当前使用的加密算法
   */
  getCurrentAlgorithm(): string {
    return SECURITY_CONFIG.ENCRYPTION.ALGORITHM;
  }

  /**
   * 获取当前密钥长度
   */
  getCurrentKeyLength(): number {
    return this.masterKey.length;
  }
}
