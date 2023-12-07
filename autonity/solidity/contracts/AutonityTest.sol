// SPDX-License-Identifier: LGPL-3.0-only

pragma solidity ^0.8.3;

import "./interfaces/IERC20.sol";
import "./Liquid.sol";
import "./Upgradeable.sol";
import "./Precompiled.sol";
import "./Autonity.sol";

/** @title Proof-of-Stake Autonity Contract */

contract AutonityTest is Autonity {

    constructor(Validator[] memory _validators,
                Config memory _config) Autonity(_validators, _config) {

    }

   function applyNewCommissionRates() public onlyProtocol {
        Autonity._applyNewCommissionRates();
   }

   function applyStakingOperations() public {
       _stakingOperations();
   }

   function testStructQuickSort() public view {
       Validator[] memory _validatorList = new Validator[](validatorList.length);
       for (uint256 i = 0; i < validatorList.length; i++) {
            Validator memory _user = validators[validatorList[i]];
            _validatorList[i] = _user;
        }
        _structQuickSort(_validatorList, 0, int(validatorList.length - 1));
   }

   function testStructQuickSortOptimized() public view {
       Validator[] memory _validatorList = new Validator[](validatorList.length);
       for (uint256 i = 0; i < validatorList.length; i++) {
            Validator memory _user = validators[validatorList[i]];
            _validatorList[i] = _user;
        }
        _structQuickSortOptimized(_validatorList, 0, int(validatorList.length - 1));
   }

   function testStructQuickSortModified() public view {
       StructForSorting[] memory _validatorList = new StructForSorting[](validatorList.length);
       for (uint256 i = 0; i < validatorList.length; i++) {
             Validator storage _user = validators[validatorList[i]];
             StructForSorting memory _item = StructForSorting(
                 _user.nodeAddress, _user.oracleAddress, _user.enode, _user.bondedStake
             );
            _validatorList[i] = _item;
        }
        _structQuickSortModified(_validatorList, 0, int(validatorList.length - 1));
   }

   function testStructQuickSortModifiedOptimized() public view {
       StructForSorting[] memory _validatorList = new StructForSorting[](validatorList.length);
       for (uint256 i = 0; i < validatorList.length; i++) {
             Validator storage _user = validators[validatorList[i]];
             StructForSorting memory _item = StructForSorting(
                 _user.nodeAddress, _user.oracleAddress, _user.enode, _user.bondedStake
             );
            _validatorList[i] = _item;
        }
        _structQuickSortModifiedOptimized(_validatorList, 0, int(validatorList.length - 1));
   }

   function testStructQuickSortWithCommittee() public view {
       CommitteeMember[] memory _validatorList = new CommitteeMember[](validatorList.length);
       for (uint256 i = 0; i < validatorList.length; i++) {
            Validator storage _user = validators[validatorList[i]];
            CommitteeMember memory _item = CommitteeMember(_user.nodeAddress, _user.bondedStake);
            _validatorList[i] = _item;
        }
        _structQuickSortWithCommittee(_validatorList, 0, int(validatorList.length - 1));
   }

   function testStructQuickSortWithCommitteeOptimized() public view {
       CommitteeMember[] memory _validatorList = new CommitteeMember[](validatorList.length);
       for (uint256 i = 0; i < validatorList.length; i++) {
            Validator storage _user = validators[validatorList[i]];
            CommitteeMember memory _item = CommitteeMember(_user.nodeAddress, _user.bondedStake);
            _validatorList[i] = _item;
        }
        _structQuickSortWithCommitteeOptimized(_validatorList, 0, int(validatorList.length - 1));
   }

   function getBondingRequest(uint256 _id) public view returns (BondingRequest memory) {
        return bondingMap[_id];
   }

   function getUnbondingRequest(uint256 _id) public view returns (UnbondingRequest memory) {
        return unbondingMap[_id];
   }

   function getTailBondingID() public view returns (uint256) {
     return tailBondingID;
   }

   function getHeadBondingID() public view returns (uint256) {
     return headBondingID;
   }

   function getLastUnlockedUnbonding() public view returns (uint256) {
     return lastUnlockedUnbonding;     
   }

   function getHeadUnbondingID() public view returns (uint256) {
     return headUnbondingID;
   }

}
