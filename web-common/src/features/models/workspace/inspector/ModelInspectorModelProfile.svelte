<script lang="ts">
  import ColumnProfile from "@rilldata/web-common/components/column-profile/ColumnProfile.svelte";
  import { useModel } from "@rilldata/web-common/features/models/selectors";
  import CollapsibleSectionTitle from "@rilldata/web-common/layout/CollapsibleSectionTitle.svelte";
  import { LIST_SLIDE_DURATION } from "@rilldata/web-common/layout/config";
  import type { V1ModelV2 } from "@rilldata/web-common/runtime-client";
  import { slide } from "svelte/transition";
  import { runtime } from "../../../../runtime-client/runtime-store";
  import References from "./References.svelte";

  export let modelName: string;

  $: modelQuery = useModel($runtime?.instanceId, modelName);
  let model: V1ModelV2;
  // refresh entry value only if the data has changed
  $: model = $modelQuery?.data?.model ?? model;

  let showColumns = true;
</script>

<div class="model-profile">
  {#if model && model?.spec?.sql?.trim()?.length}
    <References {modelName} />

    <div class="pb-4 pt-4">
      <div class=" pl-4 pr-4">
        <CollapsibleSectionTitle
          tooltipText="selected columns"
          bind:active={showColumns}
        >
          Selected columns
        </CollapsibleSectionTitle>
      </div>

      {#if showColumns}
        <div transition:slide={{ duration: LIST_SLIDE_DURATION }}>
          <ColumnProfile objectName={model?.state?.table} indentLevel={0} />
        </div>
      {/if}
    </div>
  {/if}
</div>
