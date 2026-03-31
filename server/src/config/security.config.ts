// 安全配置常量
export const SECURITY_CONFIG = {
  // 加密配置
  ENCRYPTION: {
    ALGORITHM: 'aes-256-gcm',
    KEY_LENGTH: 32, // 256位 = 32字节
    IV_LENGTH: 12, // 96位 = 12字节 (AES-GCM标准)
    AUTH_TAG_LENGTH: 16, // 128位 = 16字节 (AES-GCM标准)
    
    // 密钥轮换配置
    KEY_ROTATION_INTERVAL_DAYS: 90, // 90天轮换一次主密钥
    OLD_KEY_RETENTION_DAYS: 180, // 保留旧密钥180天用于历史数据解密
    
    // 加密策略
    ENCRYPT_SENSITIVE_FIELDS: ['password', 'secret', 'token', 'key', 'private', 'credential'],
    DATA_CLASSIFICATION_LEVELS: ['public', 'internal', 'confidential', 'restricted'],
  },

  // 安全日志配置
  LOGGING: {
    ENABLE_AUDIT_LOGS: true,
    INCLUDE_USER_ID: true,
    INCLUDE_IP_ADDRESS: true,
    INCLUDE_TIMESTAMP: true,
    MAX_LOG_RETENTION_DAYS: 90,
    ENABLE_DEEP_LOGGING: false, // 通常在生产环境中关闭，除非调试需要
  },

  // 错误处理配置
  ERROR_HANDLING: {
    HIDE_SENSITIVE_ERRORS: true, // 隐藏敏感信息不显示给客户端
    MASK_PERSONAL_INFO_IN_LOGS: true,
    LOG_SECURITY_EVENTS: true,
    USE_GENERIC_ERROR_MESSAGES: true,
  },

  // HTTP安全配置
  HTTP_SECURITY: {
    ENABLE_HSTS: true,
    ENABLE_XSS_PROTECTION: true,
    ENABLE_CONTENT_TYPE_OPTIONS: true,
    ENABLE_FRAME_OPTIONS: true,
    ENABLE_CSP: true,
    HTTPS_REDIRECT_ENABLED: true,
    STRICT_TRANSPORT_SECURITY: 'max-age=31536000; includeSubDomains',
  },

  // 输入验证配置
  INPUT_VALIDATION: {
    ENABLE_SANITIZATION: true,
    BLOCK_SQL_INJECTION: true,
    BLOCK_XSS_ATTACKS: true,
    MAX_REQUEST_SIZE: '10mb',
    WHITELIST_HOSTS: [], // 可以配置允许的主机白名单
  }
};

// 密钥管理策略
export class KeyManagementStrategy {
  static readonly DEFAULT_ROTATION_POLICY = {
    interval: 30 * 24 * 60 * 60 * 1000, // 30 days in milliseconds
    allow_old_keys_for: 60 * 24 * 60 * 60 * 1000, // 60 days to accommodate old encryptions
    backup_enabled: true,
    audit_logging_enabled: true
  };

  static readonly SECURITY_LEVELS = {
    HIGH: {
      key_size: 32,
      iterations: 100000,
      algorithm: 'aes-256-gcm'
    },
    MEDIUM: {
      key_size: 24,
      iterations: 50000,
      algorithm: 'aes-192-gcm'
    },
    LOW: {
      key_size: 16,
      iterations: 10000,
      algorithm: 'aes-128-gcm'
    }
  };
}

// 加密策略配置
export interface EncryptionPolicy {
  minKeyLength: number;
  algorithm: string;
  enforceKeyRotation: boolean;
  allowedDataTypes: string[];
  auditEncryptedOperations: boolean;
}