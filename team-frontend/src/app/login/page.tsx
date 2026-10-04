import { LoginForm } from "@/features/auth/LoginForm";

export default function LoginPage() {
  return (
    <section className="space-y-6 py-2">
      <h1 className="text-center text-2xl font-bold text-text-primary">
        Đăng nhập
      </h1>
      <LoginForm />
    </section>
  );
}
