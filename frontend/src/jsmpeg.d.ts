declare module "@cycjimmy/jsmpeg-player" {
  const JSMpeg: { Player: new (url: string, options: Record<string, unknown>) => any };
  export default JSMpeg;
}
