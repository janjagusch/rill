<script lang="ts">
  import type { EditorView } from "@codemirror/view";
  import { useQueryClient } from "@tanstack/svelte-query";
  import { parse } from "yaml";
  import YAMLEditor from "../../components/editor/YAMLEditor.svelte";
  import { createRuntimeServiceGetFile } from "../../runtime-client";
  import { runtime } from "../../runtime-client/runtime-store";
  import { saveFile } from "./actions";
  import ErrorPane from "./ErrorPane.svelte";

  export let fileName: string;

  let editor: YAMLEditor;
  let view: EditorView;
  let error: Error | undefined;

  const queryClient = useQueryClient();

  $: file = createRuntimeServiceGetFile($runtime.instanceId, fileName, {
    query: {
      // this will ensure that any changes done outside our app is pulled in.
      refetchOnWindowFocus: true,
    },
  });

  async function handleUpdate(e: CustomEvent<{ content: string }>) {
    const blob = e.detail.content;
    await saveFile(queryClient, fileName, blob);
    error = validateYAMLAndReturnError(blob);
  }

  function validateYAMLAndReturnError(blob: string): Error | undefined {
    try {
      parse(blob);
      return undefined;
    } catch (e) {
      return e;
    }
  }

  function cleanErrorMessage(message: string): string {
    return message?.replace("YAMLParseError: ", "");
  }
</script>

<div
  class="flex flex-col w-full h-full content-stretch"
  style:height={"calc(100vh - var(--header-height))"}
>
  <div class="grow bg-white overflow-y-auto">
    <div
      class="border-white w-full overflow-y-auto"
      class:border-b-hidden={error}
      class:border-red-500={error}
    >
      <YAMLEditor
        bind:this={editor}
        bind:view
        content={$file?.data?.blob || ""}
        on:update={handleUpdate}
      />
    </div>
  </div>
  {#if error}
    <ErrorPane errorMessage={cleanErrorMessage(error.message)} />
  {/if}
</div>
