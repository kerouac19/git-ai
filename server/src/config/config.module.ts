import { Module } from '@nestjs/common';
import { ConfigController } from './config.controller';
import { ConfigService } from './config.service';
import { SecurityModule } from '../security/security.module';
import { PrismaModule } from '../prisma/prisma.module';

@Module({
  imports: [
    PrismaModule,
    SecurityModule,
  ],
  controllers: [
    ConfigController,
  ],
  providers: [
    ConfigService,
  ],
  exports: [
    ConfigService,
  ],
})
export class ConfigModule {}
