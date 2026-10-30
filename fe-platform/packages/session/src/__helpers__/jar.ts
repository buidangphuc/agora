export class FakeJar {
  readonly store = new Map<string, string>();
  readonly deleted: string[] = [];
  secureProto: string | null = null;

  get = (name: string) => {
    const value = this.store.get(name);
    return value === undefined ? undefined : { name, value };
  };

  set = (name: string, value: string, _opts?: unknown) => {
    this.store.set(name, value);
  };

  delete = (name: string) => {
    this.deleted.push(name);
    this.store.delete(name);
  };

  headers = () => ({
    get: (n: string) => (n === "x-forwarded-proto" ? this.secureProto : null),
  });
}
