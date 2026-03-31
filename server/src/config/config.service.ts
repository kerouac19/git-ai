import { Injectable, Logger, NotFoundException, BadRequestException } from '@nestjs/common';
import { Prisma, Config as PrismaConfig } from '@prisma/client';
import { CreateConfigDto, UpdateConfigDto, QueryConfigDto, ConfigCategory } from './config.dto';
import { EncryptionService } from '../security/encryption.service';
import { PrismaService } from '../prisma/prisma.service';

type ConfigRecord = {
  id: string;
  key: string;
  value: string | null;
  description?: string | null;
  category: string;
  is_sensitive: boolean;
  created_at: Date;
  updated_at: Date;
};

@Injectable()
export class ConfigService {
  private readonly logger = new Logger(ConfigService.name);

  constructor(
    private readonly prisma: PrismaService,
    private readonly encryptionService: EncryptionService,
  ) {}

  /**
   * 根据键获取配置
   */
  async getConfig(key: string): Promise<ConfigRecord | null> {
    try {
      const config = await this.prisma.config.findUnique({
        where: { key },
      });

      if (!config) {
        this.logger.log(`Configuration not found: ${key}`);
        return null;
      }

      if (config.isSensitive) {
        this.logger.log(`Sensitive configuration retrieved: ${config.key}`);
      } else {
        this.logger.log(`Configuration retrieved: ${key}`);
      }

      return this.toConfigRecord(config);
    } catch (error) {
      this.logger.error(`Error retrieving configuration ${key}:`, error);
      throw error;
    }
  }

  /**
   * 获取所有配置或特定分类配置
   */
  async getAllConfigs(queryDto: QueryConfigDto): Promise<ConfigRecord[]> {
    try {
      const { category, key } = queryDto;
      const where: Prisma.ConfigWhereInput = {};

      if (category) {
        where.category = category;
      }

      if (key) {
        where.key = key;
      }

      const configs = await this.prisma.config.findMany({
        where,
        orderBy: [{ category: 'asc' }, { key: 'asc' }],
      });

      return configs.map((config) => {
        if (config.isSensitive) {
          this.logger.debug(`Retrieved sensitive configuration: ${config.key}`);
        }

        return this.toConfigRecord(config);
      });
    } catch (error) {
      this.logger.error('Error retrieving configurations:', error);
      throw error;
    }
  }

  /**
   * 创建新配置项
   */
  async createConfig(createDto: CreateConfigDto): Promise<ConfigRecord> {
    try {
      const existingConfig = await this.prisma.config.findUnique({
        where: { key: createDto.key },
      });

      if (existingConfig) {
        throw new BadRequestException(`Configuration key '${createDto.key}' already exists`);
      }

      let processedValue = createDto.value;
      if (createDto.is_sensitive) {
        try {
          processedValue = await this.encryptConfigValue(createDto.key, createDto.value);
        } catch (encryptionError) {
          this.logger.error(`Failed to encrypt sensitive config value for key: ${createDto.key}`, encryptionError);
          throw new BadRequestException('Failed to encrypt sensitive configuration value');
        }
      }

      const savedConfig = await this.prisma.config.create({
        data: {
          key: createDto.key,
          value: processedValue,
          description: createDto.description,
          category: createDto.category || ConfigCategory.GENERAL,
          isSensitive: createDto.is_sensitive || false,
        },
      });

      this.logger.log(`Configuration created: ${savedConfig.key}, Category: ${savedConfig.category}`);
      return this.toConfigRecord(savedConfig);
    } catch (error) {
      this.logger.error(`Error creating configuration ${createDto.key}:`, error);
      throw error;
    }
  }

  /**
   * 更新配置值
   */
  async updateConfig(key: string, updateDto: UpdateConfigDto): Promise<ConfigRecord> {
    try {
      const existingConfig = await this.prisma.config.findUnique({
        where: { key },
      });

      if (!existingConfig) {
        throw new NotFoundException(`Configuration with key '${key}' not found`);
      }

      const updateData: Prisma.ConfigUpdateInput = {};

      if (updateDto.description !== undefined) {
        updateData.description = updateDto.description;
      }

      if (updateDto.value !== undefined) {
        if (existingConfig.isSensitive) {
          try {
            updateData.value = await this.encryptConfigValue(key, updateDto.value);
          } catch (encryptionError) {
            this.logger.error(`Failed to encrypt sensitive config value for key: ${key}`, encryptionError);
            throw new BadRequestException('Failed to encrypt sensitive configuration value');
          }
        } else {
          updateData.value = updateDto.value;
        }
      }

      const updatedConfig = await this.prisma.config.update({
        where: { key },
        data: updateData,
      });

      this.logger.log(`Configuration updated: ${key}`);
      return this.toConfigRecord(updatedConfig);
    } catch (error) {
      this.logger.error(`Error updating configuration ${key}:`, error);
      throw error;
    }
  }

  /**
   * 删除配置项
   */
  async deleteConfig(key: string): Promise<boolean> {
    try {
      const configToDelete = await this.prisma.config.findUnique({
        where: { key },
      });

      if (!configToDelete) {
        throw new NotFoundException(`Configuration with key '${key}' not found`);
      }

      await this.prisma.config.delete({
        where: { key },
      });

      if (configToDelete.isSensitive) {
        this.logger.warn(`Sensitive configuration deleted: ${key}`);
      } else {
        this.logger.log(`Configuration deleted: ${key}`);
      }

      return true;
    } catch (error) {
      this.logger.error(`Error deleting configuration ${key}:`, error);
      throw error;
    }
  }

  /**
   * 检查配置键是否存在
   */
  async configExists(key: string): Promise<boolean> {
    try {
      const count = await this.prisma.config.count({
        where: { key },
      });
      return count > 0;
    } catch (error) {
      this.logger.error(`Error checking config existence for key ${key}:`, error);
      throw error;
    }
  }

  /**
   * 对特定键的配置值进行验证
   */
  validateConfigValue(key: string, value: any): boolean {
    try {
      if (typeof value !== 'string' && typeof value !== 'number' && typeof value !== 'boolean') {
        return false;
      }

      switch (key) {
        case 'jwt_expiration':
          return /^(\d+)(d|h|m|s)$/i.test(value.toString());

        case 'max_upload_size':
          return /^(\d+)(kb|mb|gb)$/i.test(value.toString().toLowerCase());

        case 'log_level':
          return ['error', 'warn', 'info', 'http', 'verbose', 'debug'].includes(value.toString().toLowerCase());

        case 'rate_limit_requests':
          return !isNaN(Number(value));

        default: {
          const sqlInjectionPattern = /(\b(drop|create|alter|delete|insert)\b|\-\-|;)/i;
          return !sqlInjectionPattern.test(value.toString());
        }
      }
    } catch (error) {
      this.logger.error(`Error validating config value for key ${key}:`, error);
      return false;
    }
  }

  /**
   * 批量获取配置值
   */
  async getMultipleConfig(keys: string[]): Promise<Record<string, any>> {
    try {
      const configs = await this.prisma.config.findMany({
        where: {
          key: {
            in: keys,
          },
        },
      });

      const result: Record<string, any> = {};
      configs.forEach((config) => {
        result[config.key] = config.isSensitive
          ? this.maskConfigValue(config.value)
          : config.value;
      });

      return result;
    } catch (error) {
      this.logger.error('Error getting multiple configs:', error);
      throw error;
    }
  }

  private toConfigRecord(config: PrismaConfig): ConfigRecord {
    return {
      id: config.id,
      key: config.key,
      value: config.isSensitive ? this.maskConfigValue(config.value) : config.value,
      description: config.description,
      category: config.category,
      is_sensitive: config.isSensitive,
      created_at: config.createdAt,
      updated_at: config.updatedAt,
    };
  }

  private async encryptConfigValue(key: string, value: string) {
    const encryptedValue = await this.encryptionService.encryptData(value, `config-${key}`);
    return JSON.stringify(encryptedValue);
  }

  private maskConfigValue(value: string | null) {
    if (!value) {
      return null;
    }

    return '[REDACTED]';
  }
}
