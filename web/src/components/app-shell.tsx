// SPDX-License-Identifier: GPL-2.0-only
import { createSignal, Show } from 'solid-js';
import { ProjectList } from './project-list';
import { ProjectDetail } from './project-detail';
import { ProjectCreateDialog } from './project-create-dialog';
import { WorkItemList } from './work-item-list';
import { WorkItemDetail } from './work-item-detail';
import { createProjectsStore, type Project } from '../stores/projects';
import { createVocabulariesStore } from '../stores/vocabularies';
import type { WorkItem } from '../stores/work-items';

type Screen = 'projects' | 'work-items' | 'agents' | 'audit';

export function AppShell(props: { connected: boolean }) {
  const [screen, setScreen] = createSignal<Screen>('projects');
  const [selectedProject, setSelectedProject] = createSignal<Project | null>(null);
  const [selectedWorkItem, setSelectedWorkItem] = createSignal<WorkItem | null>(null);
  const [createOpen, setCreateOpen] = createSignal(false);

  const { reload: reloadProjects, createProject } = createProjectsStore();
  const { vocabularies } = createVocabulariesStore();

  return (
    <div>
      <Show when={!props.connected}>
        <wf-banner variant="warning" headline="Flow service unreachable" />
      </Show>
      <nav>
        <wf-button on:click={() => setScreen('projects')}>Projects</wf-button>
        <wf-button on:click={() => setScreen('work-items')}>Work Items</wf-button>
        <wf-button on:click={() => setScreen('agents')}>Agents</wf-button>
        <wf-button on:click={() => setScreen('audit')}>Audit</wf-button>
      </nav>
      <main>
        <Show when={screen() === 'projects'}>
          <Show when={selectedProject()} fallback={
            <div>
              <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: var(--wf-space-md);">
                <h1 style="margin: 0;">Projects</h1>
                <wf-button on:click={() => setCreateOpen(true)}>New Project</wf-button>
              </div>
              <ProjectList onSelect={(p) => setSelectedProject(p)} />
              <ProjectCreateDialog
                open={createOpen()}
                vocabularies={vocabularies() ?? []}
                onSubmit={async (body) => {
                  await createProject(body);
                  setCreateOpen(false);
                }}
                onClose={() => setCreateOpen(false)}
              />
            </div>
          }>
            <div>
              <wf-button variant="text" on:click={() => setSelectedProject(null)}>
                ← Projects
              </wf-button>
              <ProjectDetail
                project={selectedProject()!}
                onUpdated={(p) => { setSelectedProject(p); reloadProjects(); }}
                onDeleted={() => { setSelectedProject(null); reloadProjects(); }}
              />
            </div>
          </Show>
        </Show>

        <Show when={screen() === 'work-items'}>
          <Show when={selectedProject()} fallback={
            <p style="color: var(--wf-color-text-muted);">Select a project in the Projects tab first.</p>
          }>
            <Show when={selectedWorkItem()} fallback={
              <div>
                <h2>Work Items — {selectedProject()!.name}</h2>
                <WorkItemList
                  projectId={selectedProject()!.id}
                  onSelect={(wi) => setSelectedWorkItem(wi)}
                />
              </div>
            }>
              <WorkItemDetail
                item={selectedWorkItem()!}
                onBack={() => setSelectedWorkItem(null)}
              />
            </Show>
          </Show>
        </Show>

        <Show when={screen() === 'agents'}>
          <p>Agent Pool</p>
        </Show>
        <Show when={screen() === 'audit'}>
          <p>Audit Log</p>
        </Show>
      </main>
    </div>
  );
}
