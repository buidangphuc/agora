# Tasks

## 1. Code — platform-recsys
- [x] Implement `recsys/two_tower/user_tower.py` (`UserTower`).
- [x] Implement `recsys/two_tower/item_tower.py` (`ItemTower`).
- [x] Implement `recsys/two_tower/model.py` (`TwoTowerModel` with candidate index and similarity retrieval).
- [x] Implement `recsys/two_tower/__init__.py`.
- [x] Add unit tests in `platform-recsys/tests/test_two_tower.py`.

## 2. Verification
- [x] Validate OpenSpec change (`openspec validate add-two-tower-retrieval --strict`).
- [x] Run `pytest -v tests/test_two_tower.py` in `platform-recsys/`.
