import { Injectable } from '@nestjs/common';
import { AuthorshipRecord, CommitAttribution, Prisma } from '@prisma/client';
import { AuthorshipDto, CommitAttributionDto } from './authorship.dto';
import { PrismaService } from '../prisma/prisma.service';

type AuthorshipRecordView = Omit<AuthorshipRecord, 'fileAttributions'> & {
  fileAttributions: AuthorshipDto['fileAttributions'];
};

type CommitAttributionView = Omit<
  CommitAttribution,
  'fileChanges' | 'aiContributionMetrics'
> & {
  fileChanges: CommitAttributionDto['fileChanges'];
  aiContributionMetrics: CommitAttributionDto['aiContributionMetrics'];
};

@Injectable()
export class AuthorshipService {
  constructor(private readonly prisma: PrismaService) {}

  async saveAuthorshipRecord(authorshipDto: AuthorshipDto): Promise<AuthorshipRecordView> {
    const record = await this.prisma.authorshipRecord.create({
      data: {
        userId: authorshipDto.userId,
        gitCommitHash: authorshipDto.gitCommitHash,
        fileAttributions: JSON.stringify(authorshipDto.fileAttributions),
        aiAttributionPercentage: authorshipDto.aiAttributionPercentage,
      },
    });

    return this.toAuthorshipRecordView(record);
  }

  async saveCommitAttribution(
    commitAttributionDto: CommitAttributionDto,
  ): Promise<CommitAttributionView> {
    const attribution = await this.prisma.commitAttribution.create({
      data: {
        commitHash: commitAttributionDto.commitHash,
        author: commitAttributionDto.author,
        fileChanges: JSON.stringify(commitAttributionDto.fileChanges),
        aiContributionMetrics: JSON.stringify(commitAttributionDto.aiContributionMetrics),
      },
    });

    return this.toCommitAttributionView(attribution);
  }

  async findByUserId(userId: string): Promise<AuthorshipRecordView[]> {
    const records = await this.prisma.authorshipRecord.findMany({
      where: {
        userId: {
          contains: userId,
        },
      },
      orderBy: {
        createdAt: 'desc',
      },
    });

    return records.map((record) => this.toAuthorshipRecordView(record));
  }

  async findByCommitHash(commitHash: string): Promise<AuthorshipRecordView[]> {
    const records = await this.prisma.authorshipRecord.findMany({
      where: {
        gitCommitHash: {
          contains: commitHash,
        },
      },
      orderBy: {
        createdAt: 'desc',
      },
    });

    return records.map((record) => this.toAuthorshipRecordView(record));
  }

  async findCommitAttributionByHash(
    commitHash: string,
  ): Promise<CommitAttributionView | null> {
    const record = await this.prisma.commitAttribution.findFirst({
      where: { commitHash },
    });

    return record ? this.toCommitAttributionView(record) : null;
  }

  async findAll(
    userId: string,
    limit: number = 50,
    offset: number = 0,
  ): Promise<{ records: AuthorshipRecordView[]; total: number }> {
    const [records, total] = await this.prisma.$transaction([
      this.prisma.authorshipRecord.findMany({
        where: { userId },
        orderBy: { createdAt: 'desc' },
        take: limit,
        skip: offset,
      }),
      this.prisma.authorshipRecord.count({
        where: { userId },
      }),
    ]);

    return {
      records: records.map((record) => this.toAuthorshipRecordView(record)),
      total,
    };
  }

  async mergeAuthorshipData(
    userId: string,
    newRecord: AuthorshipDto,
  ): Promise<AuthorshipRecordView> {
    const payload = {
      fileAttributions: JSON.stringify(newRecord.fileAttributions),
      aiAttributionPercentage: newRecord.aiAttributionPercentage,
    };

    const existingRecord = await this.prisma.authorshipRecord.findFirst({
      where: {
        userId,
        gitCommitHash: newRecord.gitCommitHash,
      },
    });

    if (existingRecord) {
      const updated = await this.prisma.authorshipRecord.update({
        where: { id: existingRecord.id },
        data: payload,
      });

      return this.toAuthorshipRecordView(updated);
    }

    return this.saveAuthorshipRecord({
      ...newRecord,
      userId,
    });
  }

  async getAuthorshipTimeline(userId: string, startDate?: Date, endDate?: Date) {
    const where: Prisma.AuthorshipRecordWhereInput = {
      userId,
    };

    if (startDate || endDate) {
      where.createdAt = {};
      if (startDate) {
        where.createdAt.gte = startDate;
      }
      if (endDate) {
        where.createdAt.lte = endDate;
      }
    }

    const records = await this.prisma.authorshipRecord.findMany({
      where,
      orderBy: {
        createdAt: 'desc',
      },
    });

    return records.map((record) => this.toAuthorshipRecordView(record));
  }

  private toAuthorshipRecordView(record: AuthorshipRecord): AuthorshipRecordView {
    return {
      ...record,
      fileAttributions: this.parseJsonValue(record.fileAttributions, []),
    };
  }

  private toCommitAttributionView(
    record: CommitAttribution,
  ): CommitAttributionView {
    return {
      ...record,
      fileChanges: this.parseJsonValue(record.fileChanges, {}),
      aiContributionMetrics: this.parseJsonValue(record.aiContributionMetrics, {
        aiLineCount: 0,
        totalLineCount: 0,
        aiPercentage: 0,
        tokensUsed: 0,
      }),
    };
  }

  private parseJsonValue<T>(value: string, fallback: T): T {
    try {
      return JSON.parse(value) as T;
    } catch {
      return fallback;
    }
  }
}
