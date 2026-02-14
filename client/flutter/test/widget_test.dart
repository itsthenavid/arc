import 'package:arc/app/app.dart';
import 'package:arc/core/config/app_config.dart';
import 'package:arc/core/diagnostics/backend_probe.dart';
import 'package:arc/core/diagnostics/backend_status.dart';
import 'package:arc/features/auth/data/auth_repository.dart';
import 'package:arc/features/auth/data/providers.dart';
import 'package:arc/features/auth/domain/auth_models.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('app boots into auth flow', (tester) async {
    final repo = _BootstrapOnlyRepo();

    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          appConfigProvider.overrideWithValue(
            const AppConfig(
              apiBaseUrl: 'http://127.0.0.1:8080',
              inviteOnly: true,
              webCookieMode: true,
              csrfCookieName: 'arc_csrf_token',
              csrfHeaderName: 'X-CSRF-Token',
            ),
          ),
          authRepositoryProvider.overrideWithValue(repo),
          backendProbeProvider.overrideWith(
            (ref) => BackendProbeController.disabled(
              const BackendStatusState(
                level: BackendStatusLevel.online,
                message: 'Server reachable',
              ),
            ),
          ),
        ],
        child: const ArcApp(),
      ),
    );

    await tester.pump(const Duration(milliseconds: 900));

    expect(find.text('Enter Invite'), findsOneWidget);
  });
}

class _BootstrapOnlyRepo implements AuthRepository {
  @override
  ArcSession? get currentSession => null;

  @override
  ArcUser? get currentUser => null;

  @override
  Future<void> applyProfileDraft(ProfileDraft draft) async {}

  @override
  Future<AuthBootstrapResult> bootstrap() async {
    return AuthBootstrapResult.unauthenticated();
  }

  @override
  Future<void> clearInviteToken() async {}

  @override
  Future<void> clearProfileDraft() async {}

  @override
  Future<ArcInvite> createInvite({int maxUses = 1, Duration? ttl}) {
    throw UnimplementedError();
  }

  @override
  Future<ArcAuthBundle> consumeInvite({
    required String inviteToken,
    String? username,
    String? email,
    required String password,
    required bool rememberMe,
  }) {
    throw UnimplementedError();
  }

  @override
  Future<ArcSession> ensureActiveSession({Duration skew = Duration.zero}) {
    throw UnimplementedError();
  }

  @override
  Future<ArcAuthBundle> login({
    String? username,
    String? email,
    required String password,
    required bool rememberMe,
  }) {
    throw UnimplementedError();
  }

  @override
  Future<void> logout() async {}

  @override
  Future<void> logoutAll() async {}

  @override
  Future<String?> readInviteToken() async {
    return null;
  }

  @override
  Future<ArcSession> refreshSession() {
    throw UnimplementedError();
  }

  @override
  Future<ArcUser> refreshCurrentUser() {
    throw UnimplementedError();
  }

  @override
  Future<void> saveInviteToken(String token) async {}

  @override
  Future<void> saveProfileDraft(ProfileDraft draft) async {}
}
