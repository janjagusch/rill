<script lang="ts">
  import IconSpaceFixer from "@rilldata/web-common/components/button/IconSpaceFixer.svelte";
  import CaretDownIcon from "@rilldata/web-common/components/icons/CaretDownIcon.svelte";
  import WithSelectMenu from "@rilldata/web-common/components/menu/wrappers/WithSelectMenu.svelte";
  import { getStateManagers } from "@rilldata/web-common/features/dashboards/state-managers/state-managers";
  import { useTimeControlStore } from "@rilldata/web-common/features/dashboards/time-controls/time-control-store";
  import { TIME_GRAIN } from "@rilldata/web-common/lib/time/config";
  import { isGrainBigger } from "@rilldata/web-common/lib/time/grains";
  import type { TimeGrain } from "@rilldata/web-common/lib/time/types";
  import { createEventDispatcher } from "svelte";
  import type { V1TimeGrain } from "../../../runtime-client";

  export let timeGrainOptions: TimeGrain[];
  export let minTimeGrain: V1TimeGrain;

  const dispatch = createEventDispatcher();

  const timeControlsStore = useTimeControlStore(getStateManagers());

  $: activeTimeGrain = $timeControlsStore.selectedTimeRange?.interval;
  $: activeTimeGrainLabel =
    activeTimeGrain && TIME_GRAIN[activeTimeGrain]?.label;

  $: timeGrains = timeGrainOptions
    ? timeGrainOptions
        .filter((timeGrain) => !isGrainBigger(minTimeGrain, timeGrain.grain))
        .map((timeGrain) => {
          return {
            main: timeGrain.label,
            key: timeGrain.grain,
          };
        })
    : undefined;

  const onTimeGrainSelect = (timeGrain: V1TimeGrain) => {
    dispatch("select-time-grain", { timeGrain });
  };
</script>

{#if activeTimeGrain && timeGrainOptions}
  <WithSelectMenu
    paddingTop={1}
    paddingBottom={1}
    minWidth="160px"
    distance={8}
    options={timeGrains}
    selection={{
      main: activeTimeGrainLabel,
      key: activeTimeGrain,
    }}
    on:select={(event) => onTimeGrainSelect(event.detail.key)}
    let:toggleMenu
    let:active
  >
    <button
      class:bg-gray-200={active}
      class="px-3 py-2 rounded flex flex-row gap-x-2 items-center hover:bg-gray-200 hover:dark:bg-gray-600"
      on:click={toggleMenu}
    >
      <div>
        Metric trends by <span class="font-bold">{activeTimeGrainLabel}</span>
      </div>
      <IconSpaceFixer pullRight>
        <div class="transition-transform" class:-rotate-180={active}>
          <CaretDownIcon size="14px" />
        </div>
      </IconSpaceFixer>
    </button>
  </WithSelectMenu>
{/if}
