import { fetchGlog } from "./fetcher";
import { ReplayPlayer } from "./replay";

// ---------------------------------------------------------------------------
// fetchGlog tests — uses jest.spyOn to mock the global fetch API
// ---------------------------------------------------------------------------

const SAMPLE_GLOG = [
  JSON.stringify({
    step_index: 0,
    actor_id: "bot",
    action_taken: "up",
    state_snapshot: "{}",
    reward_delta: 0,
    is_terminal: false,
  }),
  JSON.stringify({
    step_index: 1,
    actor_id: "bot",
    action_taken: "down",
    state_snapshot: "{}",
    reward_delta: 1,
    is_terminal: true,
  }),
].join("\n");

function makeResponse(body: string, ok = true, status = 200): Response {
  return {
    ok,
    status,
    statusText: ok ? "OK" : "Not Found",
    text: () => Promise.resolve(body),
  } as unknown as Response;
}

describe("fetchGlog", () => {
  afterEach(() => {
    jest.restoreAllMocks();
  });

  it("returns a ReplayPlayer populated with parsed entries on success", async () => {
    jest
      .spyOn(global, "fetch")
      .mockResolvedValue(makeResponse(SAMPLE_GLOG));

    const player = await fetchGlog("https://example.com/session.glog");

    expect(player).toBeInstanceOf(ReplayPlayer);

    // Collect entries synchronously by using fake timers
    jest.useFakeTimers();
    const received: number[] = [];
    player.onEntry = (e) => received.push(e.stepIndex);
    player.play(50);
    jest.advanceTimersByTime(100);
    jest.useRealTimers();

    expect(received).toEqual([0, 1]);
  });

  it("calls fetch with the exact URL provided", async () => {
    const spy = jest
      .spyOn(global, "fetch")
      .mockResolvedValue(makeResponse(SAMPLE_GLOG));

    await fetchGlog("https://cdn.example.com/replays/abc.glog");

    expect(spy).toHaveBeenCalledTimes(1);
    expect(spy).toHaveBeenCalledWith("https://cdn.example.com/replays/abc.glog");
  });

  it("throws an error when the response status is not OK", async () => {
    jest
      .spyOn(global, "fetch")
      .mockResolvedValue(makeResponse("Not Found", false, 404));

    await expect(
      fetchGlog("https://example.com/missing.glog")
    ).rejects.toThrow("fetchGlog: request failed with status 404 Not Found");
  });

  it("throws when fetch itself rejects (network error)", async () => {
    jest
      .spyOn(global, "fetch")
      .mockRejectedValue(new TypeError("Failed to fetch"));

    await expect(fetchGlog("https://example.com/session.glog")).rejects.toThrow(
      "Failed to fetch"
    );
  });
});
