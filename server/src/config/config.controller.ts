import { Controller, Get, Post, Patch, Delete, Body, Param, Query, Logger, HttpCode, HttpStatus, UseGuards } from '@nestjs/common';
import { ConfigService } from './config.service';
import { CreateConfigDto, UpdateConfigDto, QueryConfigDto } from './config.dto';
import { ApiTags, ApiOperation, ApiResponse, ApiParam, ApiBody } from '@nestjs/swagger';
import { JwtAuthGuard } from '../guards/jwt-auth.guard';
import { PermissionGuard } from '../guards/permission.guard';
import { Role } from '../guards/permission.guard';
import { Roles } from '../guards/roles.decorator';

@ApiTags('configuration')
@Controller('config')
export class ConfigController {
  private readonly logger = new Logger(ConfigController.name);

  constructor(private readonly configService: ConfigService) {}

  @Get('/')
  @ApiOperation({ summary: '获取所有配置参数（需管理员权限）' })
  @ApiResponse({ status: 200, description: '返回所有配置参数列表' })
  @ApiResponse({ status: 401, description: '未授权访问' })
  @ApiResponse({ status: 403, description: '禁止访问，权限不足' })
  @HttpCode(HttpStatus.OK)
  @UseGuards(JwtAuthGuard, PermissionGuard)
  @Roles(Role.ADMIN)
  async getAllConfigs(@Query() queryDto: QueryConfigDto) {
    try {
      this.logger.log('Request to get all configurations');
      const configs = await this.configService.getAllConfigs(queryDto);
      this.logger.log(`Retrieved ${configs.length} configuration items`);
      
      return {
        success: true,
        data: configs,
        message: 'Configurations retrieved successfully'
      };
    } catch (error) {
      this.logger.error('Error retrieving configurations:', error);
      throw error;
    }
  }

  @Get(':key')
  @ApiOperation({ summary: '获取特定配置参数' })
  @ApiParam({ name: 'key', description: '配置参数键名', example: 'app_name' })
  @ApiResponse({ status: 200, description: '返回指定配置参数' })
  @ApiResponse({ status: 401, description: '未授权访问' })
  @ApiResponse({ status: 404, description: '配置参数不存在' })
  @HttpCode(HttpStatus.OK)
  @UseGuards(JwtAuthGuard, PermissionGuard)
  @Roles(Role.ADMIN)
  async getConfig(@Param('key') key: string) {
    try {
      this.logger.log(`Request to get config with key: ${key}`);
      const config = await this.configService.getConfig(key);
      
      if (!config) {
        this.logger.warn(`Configuration with key '${key}' not found`);
        return {
          success: false,
          data: null,
          message: 'Configuration not found'
        };
      }
      
      return {
        success: true,
        data: config,
        message: 'Configuration retrieved successfully'
      };
    } catch (error) {
      this.logger.error(`Error retrieving config with key ${key}:`, error);
      throw error;
    }
  }

  @Post('/')
  @ApiOperation({ summary: '创建新配置参数（仅管理员）' })
  @ApiBody({ type: CreateConfigDto })
  @ApiResponse({ status: 201, description: '配置参数创建成功' })
  @ApiResponse({ status: 400, description: '输入数据无效或配置键已存在' })
  @ApiResponse({ status: 401, description: '未授权访问' })
  @ApiResponse({ status: 403, description: '禁止访问，权限不足' })
  @HttpCode(HttpStatus.CREATED)
  @UseGuards(JwtAuthGuard, PermissionGuard)
  @Roles(Role.ADMIN)
  async createConfig(@Body() createDto: CreateConfigDto) {
    try {
      this.logger.log(`Request to create configuration with key: ${createDto.key}`);
      
      // 验证配置值
      if (!this.configService.validateConfigValue(createDto.key, createDto.value)) {
        this.logger.error(`Invalid configuration value for key: ${createDto.key}`);
        return {
          success: false,
          data: null,
          message: 'Invalid configuration value'
        };
      }
      
      const config = await this.configService.createConfig(createDto);
      
      this.logger.log(`Configuration created successfully: ${createDto.key}`);
      return {
        success: true,
        data: config,
        message: 'Configuration created successfully'
      };
    } catch (error) {
      this.logger.error(`Error creating config:`, error);
      throw error;
    }
  }

  @Patch(':key')
  @ApiOperation({ summary: '更新配置参数（仅管理员）' })
  @ApiParam({ name: 'key', description: '要更新的配置参数键名' })
  @ApiBody({ type: UpdateConfigDto })
  @ApiResponse({ status: 200, description: '配置参数更新成功' })
  @ApiResponse({ status: 400, description: '输入数据无效' })
  @ApiResponse({ status: 401, description: '未授权访问' })
  @ApiResponse({ status: 404, description: '配置参数不存在' })
  @HttpCode(HttpStatus.OK)
  @UseGuards(JwtAuthGuard, PermissionGuard)
  @Roles(Role.ADMIN)
  async updateConfig(@Param('key') key: string, @Body() updateDto: UpdateConfigDto) {
    try {
      this.logger.log(`Request to update configuration with key: ${key}`);
      
      // 验证更新值
      if (updateDto.value && !this.configService.validateConfigValue(key, updateDto.value)) {
        this.logger.error(`Invalid configuration value for key: ${key}`);
        return {
          success: false,
          data: null,
          message: 'Invalid configuration value'
        };
      }
      
      const config = await this.configService.updateConfig(key, updateDto);
      
      this.logger.log(`Configuration updated successfully: ${key}`);
      return {
        success: true,
        data: config,
        message: 'Configuration updated successfully'
      };
    } catch (error) {
      this.logger.error(`Error updating config with key ${key}:`, error);
      throw error;
    }
  }

  @Delete(':key')
  @ApiOperation({ summary: '删除配置参数（仅管理员）' })
  @ApiParam({ name: 'key', description: '要删除的配置参数键名' })
  @ApiResponse({ status: 200, description: '配置参数删除成功' })
  @ApiResponse({ status: 401, description: '未授权访问' })
  @ApiResponse({ status: 404, description: '配置参数不存在' })
  @HttpCode(HttpStatus.OK)
  @UseGuards(JwtAuthGuard, PermissionGuard)
  @Roles(Role.ADMIN)
  async deleteConfig(@Param('key') key: string) {
    try {
      this.logger.log(`Request to delete configuration with key: ${key}`);
      const result = await this.configService.deleteConfig(key);
      
      if (result) {
        this.logger.log(`Configuration deleted successfully: ${key}`);
        return {
          success: true,
          data: null,
          message: 'Configuration deleted successfully'
        };
      } else {
        return {
          success: false,
          data: null,
          message: 'Configuration not found'
        };
      }
    } catch (error) {
      this.logger.error(`Error deleting config with key ${key}:`, error);
      throw error;
    }
  }
}
