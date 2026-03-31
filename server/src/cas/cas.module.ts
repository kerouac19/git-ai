import { Module } from '@nestjs/common';
import { CasController } from './cas.controller';
import { CasService } from './cas.service';
import { PrismaModule } from '../prisma/prisma.module';

@Module({
  imports: [PrismaModule],
  controllers: [CasController],
  providers: [CasService],
  exports: [CasService],
})
export class CasModule {}
