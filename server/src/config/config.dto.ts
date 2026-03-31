import { IsString, IsNotEmpty, IsOptional, IsBoolean, IsEnum, IsObject } from 'class-validator';
import { ApiProperty } from '@nestjs/swagger';
import { Transform } from 'class-transformer';

export enum ConfigCategory {
  SECURITY = 'security',
  GENERAL = 'general',
  AUTH = 'auth',
  FEATURES = 'features',
  INTEGRATIONS = 'integrations',
}

export class CreateConfigDto {
  @ApiProperty({ description: '配置键名', example: 'jwt_expiration' })
  @IsString()
  @IsNotEmpty()
  key: string;

  @ApiProperty({ description: '配置值', example: '7d' })
  @IsNotEmpty()
  value: string;

  @ApiProperty({ description: '配置项描述', required: false })
  @IsOptional()
  @IsString()
  description?: string;

  @ApiProperty({ 
    description: '配置分类', 
    enum: ConfigCategory,
    required: false,
    example: ConfigCategory.GENERAL
  })
  @IsOptional()
  @IsEnum(ConfigCategory)
  category?: ConfigCategory;

  @ApiProperty({ 
    description: '是否为敏感配置', 
    required: false,
    example: false 
  })
  @IsOptional()
  @Transform(({ value }) => value === true || value === 'true')
  @IsBoolean()
  is_sensitive?: boolean;
}

export class UpdateConfigDto {
  @ApiProperty({ description: '配置值', required: false, example: '14d' })
  @IsOptional()
  value?: string;

  @ApiProperty({ description: '配置项描述', required: false })
  @IsOptional()
  @IsString()
  description?: string;

  @ApiProperty({ 
    description: '是否已加密', 
    required: false,
    example: false 
  })
  @IsOptional()
  @Transform(({ value }) => value === true || value === 'true')
  @IsBoolean()
  encrypted?: boolean;
}

export class QueryConfigDto {
  @ApiProperty({ description: '配置分类过滤', required: false })
  @IsOptional()
  @IsString()
  category?: string;

  @ApiProperty({ description: '配置键名过滤', required: false })
  @IsOptional()
  @IsString()
  key?: string;
}