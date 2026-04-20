// SPDX-License-Identifier: GPL-2.0-only
import { createResource } from 'solid-js';
import { flowAPI } from '../client';

export interface Vocabulary {
  id: string;
  name: string;
  description?: string;
}

export function createVocabulariesStore() {
  const [vocabularies] = createResource<Vocabulary[]>(
    () => flowAPI.get<Vocabulary[]>('/v1/vocabularies')
  );
  return { vocabularies };
}
