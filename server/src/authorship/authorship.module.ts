import { Module } from '@nestjs/common';
import { AuthorshipController } from './authorship.controller';
import { AuthorshipService } from './authorship.service';
import { PrismaModule } from '../prisma/prisma.module';

@Module({
  imports: [PrismaModule],
  controllers: [AuthorshipController],
  providers: [AuthorshipService],
  exports: [AuthorshipService],
})
export class AuthorshipModule {}
