// SPDX-License-Identifier: GPL-2.0-only
import { flowAPI } from '../client';

export interface Bot {
  id: string;
  project_id: string;
  passport_api_key_id?: string;
  hive_role_assignments?: string[];
  created_at: string;
  updated_at: string;
}

export interface BotPlaintextOutput {
  bot: Bot;
  plaintext_api_key: string;
}

export interface CreateBotBody {
  hive_role_assignments?: string[];
  bring_your_own_key?: boolean;
  passport_api_key?: string;
  passport_api_key_id?: string;
}

export async function createBot(projectId: string, body: CreateBotBody): Promise<BotPlaintextOutput> {
  return flowAPI.post<BotPlaintextOutput>(`/v1/projects/${projectId}/bot`, body);
}

export async function getBot(projectId: string): Promise<Bot | null> {
  try {
    return await flowAPI.get<Bot>(`/v1/projects/${projectId}/bot`);
  } catch (e: any) {
    if (e?.status === 404) return null;
    throw e;
  }
}

export async function deleteBot(projectId: string): Promise<void> {
  await flowAPI.delete(`/v1/projects/${projectId}/bot`);
}

export async function rotateBotKey(projectId: string): Promise<BotPlaintextOutput> {
  return flowAPI.post<BotPlaintextOutput>(`/v1/projects/${projectId}/bot/rotate-key`);
}
