import { Injectable } from '@nestjs/common';
import * as crypto from 'crypto';
import * as zlib from 'zlib';
import { promisify } from 'util';
import { PrismaService } from '../prisma/prisma.service';
import { resolveCasEncryptionKey } from '../security/runtime-secrets';

const deflate = promisify(zlib.deflate);
const inflate = promisify(zlib.inflate);

@Injectable()
export class CasService {
  constructor(private readonly prisma: PrismaService) {}

  async uploadObject(
    providedHash: string,
    content: unknown,
    _metadata?: Record<string, string>,
  ): Promise<string> {
    const hash = providedHash.trim().toLowerCase();
    const existingEntry = await this.prisma.casEntry.findUnique({
      where: { hash },
    });

    if (existingEntry) {
      return hash;
    }

    const serializedContent = JSON.stringify(content);
    const compressedContent = await deflate(serializedContent);
    const encryptedContent = await this.encryptContent(compressedContent.toString('base64'));

    await this.prisma.casEntry.create({
      data: {
        hash,
        encryptedContent,
        contentType: 'application/json',
      },
    });

    return hash;
  }

  async uploadObjects(
    objects: Array<{
      hash?: string;
      content?: unknown;
      metadata?: Record<string, string>;
    }>,
  ) {
    const results: Array<{ hash: string; status: string; error?: string }> = [];

    for (const object of objects) {
      const hash = object.hash?.trim().toLowerCase();
      if (!hash) {
        results.push({
          hash: object.hash || '',
          status: 'error',
          error: 'hash is required',
        });
        continue;
      }

      if (typeof object.content === 'undefined') {
        results.push({
          hash,
          status: 'error',
          error: 'content is required',
        });
        continue;
      }

      try {
        await this.uploadObject(hash, object.content, object.metadata);
        results.push({
          hash,
          status: 'ok',
        });
      } catch (error: any) {
        results.push({
          hash,
          status: 'error',
          error: error?.message || 'Unknown error',
        });
      }
    }

    const successCount = results.filter((result) => result.status === 'ok').length;
    return {
      results,
      success_count: successCount,
      failure_count: results.length - successCount,
    };
  }

  async uploadContent(content: string, contentType: string = 'text/plain'): Promise<string> {
    // 压缩内容
    const compressedContent = await deflate(content);
    
    // 计算内容的SHA256哈希值
    const hash = crypto.createHash('sha256');
    hash.update(compressedContent);
    const contentHash = hash.digest('hex');

    // 加密内容
    const encryptedContent = await this.encryptContent(compressedContent.toString('base64'));

    // 检查该哈希是否已经存在
    const existingEntry = await this.prisma.casEntry.findUnique({
      where: { hash: contentHash },
    });

    if (existingEntry) {
      // 如果已经存在，则返回现有的哈希，不用重复存储
      return contentHash;
    }

    // 保存到数据库
    await this.prisma.casEntry.create({
      data: {
        hash: contentHash,
        encryptedContent,
        contentType,
      },
    });

    return contentHash;
  }

  async readContent(hash: string): Promise<{ content: string; contentType: string } | null> {
    const casEntry = await this.prisma.casEntry.findUnique({
      where: { hash },
    });

    if (!casEntry) {
      return null;
    }

    // 解密内容
    const decryptedCompressedContent = await this.decryptContent(casEntry.encryptedContent);
    
    // 解压缩内容
    const decompressedBuffer = await inflate(Buffer.from(decryptedCompressedContent, 'base64'));
    
    return {
      content: decompressedBuffer.toString(),
      contentType: casEntry.contentType,
    };
  }

  async readObject(hash: string): Promise<unknown | null> {
    const result = await this.readContent(hash.trim().toLowerCase());
    if (!result) {
      return null;
    }

    try {
      return JSON.parse(result.content);
    } catch {
      return null;
    }
  }

  async readObjects(hashes: string[]) {
    const results: Array<{
      hash: string;
      status: string;
      content?: unknown;
      error?: string;
    }> = [];

    for (const originalHash of hashes) {
      const hash = originalHash.trim().toLowerCase();
      if (!hash) {
        continue;
      }

      try {
        const content = await this.readObject(hash);
        if (content === null) {
          results.push({
            hash,
            status: 'error',
            error: 'Content not found',
          });
          continue;
        }

        results.push({
          hash,
          status: 'ok',
          content,
        });
      } catch (error: any) {
        results.push({
          hash,
          status: 'error',
          error: error?.message || 'Unknown error',
        });
      }
    }

    const successCount = results.filter((result) => result.status === 'ok').length;
    return {
      results,
      success_count: successCount,
      failure_count: results.length - successCount,
    };
  }

  private async encryptContent(content: string): Promise<string> {
    // 生产环境要求显式配置，开发环境回退到进程内临时密钥。
    const secretKey = resolveCasEncryptionKey();
    const salt = 'GitAISalt'; // 在实际使用中，这应该是随机生成的盐
    
    // 使用 Scrypt 派生出合适的密钥长度
    const key = crypto.scryptSync(secretKey, salt, 32);
    const iv = crypto.randomBytes(16); // 初始化向量
    const cipher = crypto.createCipheriv('aes-256-gcm', key, iv);

    let encrypted = cipher.update(content, 'utf8', 'hex');
    encrypted += cipher.final('hex');
    
    const authTag = cipher.getAuthTag(); // GCM模式需要的身份验证标签

    // 将 IV, 认证标签和加密内容组合在一起
    return `${iv.toString('hex')}:${authTag.toString('hex')}:${encrypted}`;
  }

  private async decryptContent(encryptedContent: string): Promise<string> {
    try {
      // 分离 IV、认证标签和加密数据
      const parts = encryptedContent.split(':');
      if (parts.length !== 3) {
        throw new Error('Invalid encrypted content format');
      }

      const iv = Buffer.from(parts[0], 'hex');
      const authTag = Buffer.from(parts[1], 'hex');
      const encryptedData = parts[2];

      const secretKey = resolveCasEncryptionKey();
      const salt = 'GitAISalt'; // 必须是与加密时使用的相同盐
      
      // 使用 Scrypt 派生出合适的密钥长度
      const key = crypto.scryptSync(secretKey, salt, 32);
      
      const decipher = crypto.createDecipheriv('aes-256-gcm', key, iv);
      decipher.setAuthTag(authTag);

      let decrypted = decipher.update(encryptedData, 'hex', 'utf8');
      decrypted += decipher.final('utf8');

      return decrypted;
    } catch (error) {
      console.error('Decryption failed:', error);
      throw new Error('Failed to decrypt content');
    }
  }
}
