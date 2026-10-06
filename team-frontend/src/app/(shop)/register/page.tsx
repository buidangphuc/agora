import { RegisterForm } from "@/features/auth/RegisterForm";

export default function RegisterPage() {
  return (
    <section className="space-y-6 py-2">
      <h1 className="text-center text-2xl font-bold text-text-primary">
        Tạo tài khoản
      </h1>
      <RegisterForm />
    </section>
  );
}
