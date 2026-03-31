import {
  BadRequestException,
  Body,
  Controller,
  Get,
  Headers,
  HttpCode,
  HttpException,
  HttpStatus,
  Post,
  Query,
  Req,
  Request,
  UnauthorizedException,
  UploadedFile,
  UseGuards,
  UseInterceptors,
} from '@nestjs/common';
import { FileInterceptor } from '@nestjs/platform-express';
import { DashboardService } from '../dashboard/dashboard.service';
import { AuthorshipService } from '../authorship/authorship.service';
import { CasService } from '../cas/cas.service';
import { CompatibilityAuthService } from '../auth/compatibility-auth.service';
import { PrismaService } from '../prisma/prisma.service';
import { JwtAuthGuard } from '../guards/jwt-auth.guard';
import { MetricsService } from '../metrics/metrics.service';

interface AuthenticatedUser {
  id?: string;
  username?: string;
  email?: string;
  role?: string;
  personal_org_id?: string;
  orgs?: Array<Record<string, unknown>>;
}

interface CasUploadObjectPayload {
  hash?: string;
  content?: unknown;
  metadata?: Record<string, string>;
}

@Controller()
export class CompatibilityController {
  constructor(
    private readonly dashboardService: DashboardService,
    private readonly authorshipService: AuthorshipService,
    private readonly casService: CasService,
    private readonly compatibilityAuthService: CompatibilityAuthService,
    private readonly prisma: PrismaService,
    private readonly metricsService: MetricsService,
  ) {}

  @Get('status')
  async getStatus() {
    const publicStats = await this.dashboardService.getPublicStats();

    return {
      status: 'ok',
      service: 'git-ai-private-deploy-server',
      version: process.env.npm_package_version || '1.0.0',
      modules: ['authorship', 'cas', 'dashboard', 'config'],
      publicStats,
    };
  }

  @Get('version')
  getVersion() {
    return {
      version: process.env.npm_package_version || '1.0.0',
      service: 'git-ai-private-deploy-server',
    };
  }

  @Get('health')
  async getApiHealth() {
    return {
      status: 'ok',
      timestamp: new Date().toISOString(),
    };
  }

  @Get('health/database')
  async getDatabaseHealth() {
    try {
      await this.prisma.$queryRaw`SELECT 1`;
      return {
        status: 'ok',
        database: 'connected',
      };
    } catch (error: any) {
      throw new HttpException(
        {
          status: 'error',
          database: 'disconnected',
          message: error?.message || 'Database connectivity check failed',
        },
        HttpStatus.SERVICE_UNAVAILABLE,
      );
    }
  }

  @Get('me')
  @UseGuards(JwtAuthGuard)
  async getMe(
    @Req() req: { user?: AuthenticatedUser },
  ) {
    const user = req.user;
    const userId = typeof user?.id === 'string' ? user.id : undefined;
    if (!userId) {
      throw new UnauthorizedException('Authenticated user id is required');
    }

    const dashboard = await this.dashboardService.getDashboardStats(userId);
    const authorship = await this.authorshipService.findAll(userId, 10, 0);

    return {
      success: true,
      user: {
        id: userId,
        email: user.email || null,
        name: user.username || null,
        role: user.role || 'user',
      },
      dashboard,
      recentAuthorship: authorship.records,
      totalAuthorshipRecords: authorship.total,
    };
  }

  @Post(['worker/oauth/device/code', 'workers/oauth/device/code'])
  @HttpCode(HttpStatus.OK)
  async startDeviceFlow(@Req() req: { protocol?: string; get?: (name: string) => string | undefined }) {
    const host = req.get?.('host') || 'localhost:3000';
    const protocol = req.protocol || 'http';
    const baseUrl = `${protocol}://${host}`;
    return this.compatibilityAuthService.startDeviceFlow(baseUrl);
  }

  @Post(['worker/oauth/token', 'workers/oauth/token'])
  @HttpCode(HttpStatus.OK)
  async exchangeOAuthToken(@Body() body: Record<string, unknown>) {
    const grantType =
      typeof body.grant_type === 'string' ? body.grant_type : undefined;

    if (!grantType) {
      throw new HttpException(
        {
          error: 'invalid_request',
          error_description: 'grant_type is required',
        },
        HttpStatus.BAD_REQUEST,
      );
    }

    switch (grantType) {
      case 'urn:ietf:params:oauth:grant-type:device_code': {
        const deviceCode = typeof body.device_code === 'string' ? body.device_code : '';
        const response = await this.compatibilityAuthService.exchangeDeviceCode(deviceCode);
        return this.wrapOAuthResponse(response);
      }
      case 'refresh_token': {
        const refreshToken =
          typeof body.refresh_token === 'string' ? body.refresh_token : '';
        const response =
          this.compatibilityAuthService.exchangeRefreshToken(refreshToken);
        return this.wrapOAuthResponse(response);
      }
      case 'install_nonce': {
        const installNonce =
          typeof body.install_nonce === 'string' ? body.install_nonce : '';
        const response =
          this.compatibilityAuthService.exchangeInstallNonce(installNonce);
        return this.wrapOAuthResponse(response);
      }
      default:
        throw new HttpException(
          {
            error: 'unsupported_grant_type',
            error_description: `Unsupported grant_type: ${grantType}`,
          },
          HttpStatus.BAD_REQUEST,
        );
    }
  }

  @Post(['worker/metrics/upload', 'workers/metrics/upload'])
  @HttpCode(HttpStatus.OK)
  @UseGuards(JwtAuthGuard)
  async uploadWorkerMetrics(
    @Request() req: { user?: AuthenticatedUser; headers?: Record<string, string | string[] | undefined> },
    @Headers('x-distinct-id') distinctId?: string,
    @Body() body?: Record<string, unknown>,
  ) {
    const userId = typeof req.user?.id === 'string' ? req.user.id : undefined;
    if (!userId) {
      throw new UnauthorizedException('Authenticated user id is required');
    }

    const payload = this.metricsService.validateBatchShape(body || {});
    return this.metricsService.uploadBatch(userId, distinctId, payload);
  }

  @Post(['worker/cas/upload', 'workers/cas/upload'])
  @HttpCode(HttpStatus.OK)
  @UseGuards(JwtAuthGuard)
  @UseInterceptors(FileInterceptor('file'))
  async uploadWorkerCas(
    @Body() body?: { objects?: CasUploadObjectPayload[] },
    @UploadedFile() file?: { buffer?: Buffer; mimetype?: string },
    @Query('contentType') queryContentType?: string,
  ) {
    if (Array.isArray(body?.objects)) {
      return this.casService.uploadObjects(body.objects);
    }

    const content = file?.buffer?.toString('utf8');
    if (!content) {
      throw new BadRequestException(
        'Either JSON body "objects" or multipart file field "file" is required',
      );
    }

    const contentType = queryContentType || file.mimetype || 'application/octet-stream';
    const hash = await this.casService.uploadContent(content, contentType);

    return {
      success: true,
      object_id: hash,
      hash,
      contentType,
      message: 'Content uploaded successfully',
    };
  }

  @Get(['worker/cas', 'workers/cas'])
  @UseGuards(JwtAuthGuard)
  async readWorkerCas(@Query('hashes') hashes?: string) {
    if (!hashes?.trim()) {
      throw new BadRequestException('Query parameter "hashes" is required');
    }

    const hashList = hashes
      .split(',')
      .map((hash) => hash.trim())
      .filter(Boolean);

    if (hashList.length === 0) {
      throw new BadRequestException('Query parameter "hashes" is required');
    }

    if (hashList.length > 100) {
      throw new BadRequestException('A maximum of 100 hashes is supported per request');
    }

    return this.casService.readObjects(hashList);
  }

  @Get(['worker/cas/checkout', 'workers/cas/checkout'])
  @UseGuards(JwtAuthGuard)
  async checkoutWorkerCas(@Query('id') id?: string, @Query('hash') hash?: string) {
    const targetHash = id || hash;
    if (!targetHash) {
      throw new BadRequestException('Query parameter "id" or "hash" is required');
    }

    const result = await this.casService.readContent(targetHash);
    if (!result) {
      throw new HttpException('Content not found', HttpStatus.NOT_FOUND);
    }

    return {
      success: true,
      object_id: targetHash,
      hash: targetHash,
      content: result.content,
      contentType: result.contentType,
    };
  }

  private wrapOAuthResponse(response: Record<string, unknown>) {
    if (typeof response.error === 'string') {
      throw new HttpException(response, HttpStatus.BAD_REQUEST);
    }
    return response;
  }
}
